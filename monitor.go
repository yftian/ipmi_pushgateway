package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"github.com/cdevr/WapSNMP"
	log "github.com/cihub/seelog"
	"github.com/jinzhu/configor"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"
	"github.com/robfig/cron/v3"
	"github.com/takama/daemon"
	"io/ioutil"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	// name of the service
	name        = "ipmi_job"
	description = "Ipmi job service example"

	READ_COMM  = "public"
	WRITE_COMM = "private"
)

// Service is the daemon service struct
type Service struct {
	daemon.Daemon
}

type Config struct {
	Global struct {
		Pushgateway string
		IPMIJob     string
		SNMPJob     string
		Interval    int
		Wait        int
		Driver      string
		Type        []string
	}

	Ipmi []struct {
		Host string
		User string
		Pwd  string
	}
}

type Snmp struct {
	Lcp_ips  []string
	Lcp_oids []string

	Cool_ips  []string
	Cool_oids []string

	Pdu_ips     []string
	Pdu_am_oids []string
	Pdu_on_oids []string
}

type sensorData struct {
	ID    int64
	Name  string
	Type  string
	State string
	Value float64
	Unit  string
	Event string
}

type ipmiFiled struct {
	Name   string
	Status string
}

var (
	config = Config{}
	snmp   = Snmp{}
)

func init() {
	configor.Load(&config, "./conf/config.yml")
	//configor.Load(&snmp, "./conf/snmp.yml")

	//log section
	defer log.Flush()
	logger, err := log.LoggerFromConfigAsFile("./conf/logconf.xml")
	if err != nil {
		log.Errorf("parse config.xml error: %v", err)
		return
	}
	log.ReplaceLogger(logger)

}

func readFile(filename string) ([]byte,error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Error("File reading error", err.Error())
	}
	return data,err
}

func splitMonitoringOutput(impiOutput []byte) ([]sensorData, error) {
	var result []sensorData

	r := csv.NewReader(bytes.NewReader(impiOutput))
	records, err := r.ReadAll()
	for _, line := range records {
		//line = strings.Fields(line[0])
		line = strings.Split(line[0], "|")

		for i := 0; i < len(line); i++ {
			line[i] = strings.Trim(line[i], " ")
		}

		var data sensorData
		data.ID, err = strconv.ParseInt(line[0], 10, 64)
		if err != nil {
			continue
		}
		if len(strings.Fields(line[1])) > 1 {
			data.Name = strings.ReplaceAll(line[1], " ", "_")
			data.Name = strings.ReplaceAll(data.Name, "/", "")
		} else {
			data.Name = line[1]
		}
		if strings.Index(data.Name, "-") == 2 {
			data.Name = data.Name[3:]
			data.Name = strings.ReplaceAll(data.Name, "-", "_")
		}

		data.Type = line[2]
		data.State = line[3]
		value := line[4]
		if value != "N/A" {
			data.Value, err = strconv.ParseFloat(value, 64)
			if err != nil {
				return result, err
			}
		} else {
			data.Value = math.NaN()
		}

		data.Unit = line[5]
		data.Event = strings.Trim(line[6], "'")

		result = append(result, data)
	}
	return result, err
}

func getState(str string, subStr string) float64 {
	if strings.Contains(str, subStr) {
		return 1
	}
	return 0
}

func execute(name string, args []string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	//log.Info("cmd:", cmd.Args)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		log.Error(fmt.Sprint(err) + ":" + stderr.String())
	}
	return out.Bytes(), err
}

func splitBaseOutput(output []byte) ([]ipmiFiled, error) {
	var chass []ipmiFiled
	r := csv.NewReader(bytes.NewReader(output))
	records, err := r.ReadAll()
	for _, line := range records {
		line = strings.Split(line[0], ":")

		for i := 0; i < len(line); i++ {
			line[i] = strings.Trim(line[i], " ")
		}

		var cha ipmiFiled
		if len(strings.Fields(line[0])) > 1 {
			cha.Name = strings.ReplaceAll(line[0], " ", "_")
		} else {
			cha.Name = line[1]
		}
		cha.Status = line[1]
		chass = append(chass, cha)
	}
	return chass, err
}

//ipmimonitoring -D LAN_2_0 -h remote_ip -u username -p password
//ipmi-chassis -D LAN_2_0 -h remote_ip -u username -p password --get-status

func collectMonitoring(index int, Config Config) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*time.Duration(config.Global.Wait))
	defer cancel()

	var pushFlag bool
	for _, Mclass := range Config.Global.Type {
		switch Mclass {
		case "ipmimonitoring":
			pusher := push.New(Config.Global.Pushgateway, Config.Global.IPMIJob)
			//output,err := readFile("./file/hpIPMI.txt")
			output, err := execute("ipmimonitoring", []string{
				"-D", Config.Global.Driver,
				"-h", Config.Ipmi[index].Host,
				"-u", Config.Ipmi[index].User,
				"-p", Config.Ipmi[index].Pwd})
			if err != nil {
				log.Error(Config.Ipmi[index].Host, err.Error())
				ipmiErrGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
					Name: "MonitorMessage",
					Help: "",
				}, []string{"Host", "State", "message"})
				ipmiErrGauge.WithLabelValues(Config.Ipmi[index].Host, "Error",err.Error())
				pusher.Collector(ipmiErrGauge)
			} else {
				results, err := splitMonitoringOutput(output)
				if err != nil {
					log.Error(Config.Ipmi[index].Host, err.Error())
					ipmiErrGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
						Name: "MonitorMessage",
						Help: "",
					}, []string{"Host", "State", "message"})
					ipmiErrGauge.WithLabelValues(Config.Ipmi[index].Host, "Error",err.Error())
					pusher.Collector(ipmiErrGauge)
				} else {
					for _, data := range results {
						var state float64
						switch data.State {
						case "Nominal":
							state = 0
						case "Warning":
							state = 1
						case "Critical":
							state = 2
						case "N/A":
							state = math.NaN()
						default:
							state = math.NaN()
							log.Error(Config.Ipmi[index].Host, data.State)
						}

						ipmiGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
							Name: data.Name,
							Help: "help..",
						}, []string{"Name", "Host", "Event", "State", "Type", "Unit", "Id"})
						ipmiGaugeState := prometheus.NewGaugeVec(prometheus.GaugeOpts{
							Name: data.Name + "_state",
							Help: "help..",
						}, []string{"Name", "Host", "Event", "State", "Type", "Unit", "Id"})
						ipmiGauge.WithLabelValues(data.Name, config.Ipmi[index].Host, data.Event, data.State, data.Type, data.Unit, strconv.FormatInt(data.ID, 10)).Set(data.Value)
						ipmiGaugeState.WithLabelValues(data.Name, config.Ipmi[index].Host, data.Event, data.State, data.Type, data.Unit, strconv.FormatInt(data.ID, 10)).Set(state)
						pusher.Collector(ipmiGauge)
						pusher.Collector(ipmiGaugeState)
					}
				}
			}

			if err := pusher.Grouping("instance", "ipmi").Push(); err != nil {
				//log.Error("Could not push completion time to Pushgateway:", err)
				log.Error(config.Ipmi[index].Host, err)
			} else {
				pushFlag = true
				log.Info("ipmi push success", config.Ipmi[index].Host)
			}
		case "ipmi-chassis":
			pusher := push.New(Config.Global.Pushgateway, Config.Global.IPMIJob)
			//output,err := readFile("./file/sugonClass.txt")
			output, err := execute("ipmi-chassis", []string{
				"-D", Config.Global.Driver,
				"-h", Config.Ipmi[index].Host,
				"-u", Config.Ipmi[index].User,
				"-p", Config.Ipmi[index].Pwd,
				"--get-status"})
			if err != nil {
				log.Error(Config.Ipmi[index].Host, err.Error())
				ipmiErrGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
					Name: "ChassisMessage",
					Help: "",
				}, []string{"Host", "State", "message"})
				ipmiErrGauge.WithLabelValues(Config.Ipmi[index].Host, "Error",err.Error())
				pusher.Collector(ipmiErrGauge)
			} else {
				chass, err := splitBaseOutput(output)
				if err != nil {
					log.Error(Config.Ipmi[index].Host, err.Error())
					ipmiErrGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
						Name: "ChassisMessage",
						Help: "",
					}, []string{"Host", "State", "message"})
					ipmiErrGauge.WithLabelValues(Config.Ipmi[index].Host, "Error",err.Error())
					pusher.Collector(ipmiErrGauge)
				} else {
					for _, cha := range chass {
						switch cha.Name {
						case "System_Power":
							chaGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
								Name: cha.Name,
								Help: "",
							}, []string{"Host"})
							chaGauge.WithLabelValues(Config.Ipmi[index].Host).Set(getState(cha.Status, "on"))
							pusher.Collector(chaGauge)
						case "Power_fault":
							chaGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
								Name: cha.Name,
								Help: "",
							}, []string{"Host"})
							chaGauge.WithLabelValues(Config.Ipmi[index].Host).Set(getState(cha.Status, "false"))
							pusher.Collector(chaGauge)
						case "Drive_Fault":
							chaGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
								Name: cha.Name,
								Help: "",
							}, []string{"Host"})
							chaGauge.WithLabelValues(Config.Ipmi[index].Host).Set(getState(cha.Status, "false"))
							pusher.Collector(chaGauge)
						case "Cooling/fan_fault":
							chaGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
								Name: "fan_fault",
								Help: "",
							}, []string{"Host"})
							chaGauge.WithLabelValues(Config.Ipmi[index].Host).Set(getState(cha.Status, "false"))
							pusher.Collector(chaGauge)
						}
					}
				}
			}
			if err := pusher.Grouping("instance", "chassis").Push(); err != nil {
				log.Error("Could not push completion to Pushgateway:", config.Ipmi[index].Host, err)
				return
			} else {
				pushFlag = true
				log.Info("chassis push success:", config.Ipmi[index].Host)
			}
		case "ipmi-dcmi":
			pusher := push.New(Config.Global.Pushgateway, Config.Global.IPMIJob)
			//output,err := readFile("./file/hpDcmi.txt")
			output, err := execute("ipmi-dcmi", []string{
				"-D", Config.Global.Driver,
				"-h", Config.Ipmi[index].Host,
				"-u", Config.Ipmi[index].User,
				"-p", Config.Ipmi[index].Pwd,
				"--get-system-power-statistics"})
			if err != nil {
				log.Error(Config.Ipmi[index].Host, err.Error())
				ipmiErrGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
					Name: "DcmiMessage",
					Help: "",
				}, []string{"Host", "State", "message"})
				ipmiErrGauge.WithLabelValues(Config.Ipmi[index].Host, "Error",err.Error())
				pusher.Collector(ipmiErrGauge)
			} else {
				dcmis,err := splitBaseOutput(output)
				if err != nil {
					log.Error(Config.Ipmi[index].Host, err.Error())
					ipmiErrGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
						Name: "DcmiMessage",
						Help: "",
					}, []string{"Host", "State", "message"})
					ipmiErrGauge.WithLabelValues(Config.Ipmi[index].Host, "Error",err.Error())
					pusher.Collector(ipmiErrGauge)
				} else {
					for _, dcmi := range dcmis {
						switch dcmi.Name {
						case "Current_Power":
							dcmiGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
								Name: "Current_Power",
								Help: "",
							}, []string{"Host","Type"})
							values := strings.Split(dcmi.Status," ")
							val,err := strconv.ParseFloat(values[0], 64)
							if err != nil {
								log.Error(Config.Ipmi[index].Host, err.Error())
							}
							dcmiGauge.WithLabelValues(Config.Ipmi[index].Host,values[1]).Set(val)
							pusher.Collector(dcmiGauge)
						case "Minimum_Power_over_sampling_duration":
							dcmiGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
								Name: "Minimum_Power_over_sampling_duration",
								Help: "",
							}, []string{"Host","Type"})
							values := strings.Split(dcmi.Status," ")
							val,err := strconv.ParseFloat(values[0], 64)
							if err != nil {
								log.Error(Config.Ipmi[index].Host, err.Error())
							}
							dcmiGauge.WithLabelValues(Config.Ipmi[index].Host,values[1]).Set(val)
							pusher.Collector(dcmiGauge)
						case "Maximum_Power_over_sampling_duration":
							dcmiGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
								Name: "Maximum_Power_over_sampling_duration",
								Help: "",
							}, []string{"Host","Type"})
							values := strings.Split(dcmi.Status," ")
							val,err := strconv.ParseFloat(values[0], 64)
							if err != nil {
								log.Error(Config.Ipmi[index].Host, err.Error())
							}
							dcmiGauge.WithLabelValues(Config.Ipmi[index].Host,values[1]).Set(val)
							pusher.Collector(dcmiGauge)
						case "Average_Power_over_sampling_duration":
							dcmiGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
								Name: "Average_Power_over_sampling_duration",
								Help: "",
							}, []string{"Host","Type"})
							values := strings.Split(dcmi.Status," ")
							val,err := strconv.ParseFloat(values[0], 64)
							if err != nil {
								log.Error(Config.Ipmi[index].Host, err.Error())
							}
							dcmiGauge.WithLabelValues(Config.Ipmi[index].Host,values[1]).Set(val)
							pusher.Collector(dcmiGauge)
						case "Time_Stamp":
							dcmiGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
								Name: "Time_Stamp",
								Help: "",
							}, []string{"Host","Type"})
							if err != nil {
								log.Error(Config.Ipmi[index].Host, err.Error())
							}
							dcmiGauge.WithLabelValues(Config.Ipmi[index].Host,dcmi.Status).Set(0)
							pusher.Collector(dcmiGauge)
						case "Statistics_reporting_time_period":
							dcmiGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
								Name: "Statistics_reporting_time_period",
								Help: "",
							}, []string{"Host","Type"})
							values := strings.Split(dcmi.Status," ")
							val,err := strconv.ParseFloat(values[0], 64)
							if err != nil {
								log.Error(Config.Ipmi[index].Host, err.Error())
							}
							dcmiGauge.WithLabelValues(Config.Ipmi[index].Host,values[1]).Set(val)
							pusher.Collector(dcmiGauge)
						case "Power_Measurement":
							dcmiGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
								Name: "Power_Measurement",
								Help: "",
							}, []string{"Host"})
							if err != nil {
								log.Error(Config.Ipmi[index].Host, err.Error())
							}
							dcmiGauge.WithLabelValues(Config.Ipmi[index].Host).Set(getState(dcmi.Status,"Active"))
							pusher.Collector(dcmiGauge)
						}
					}
				}


			}
			if err := pusher.Grouping("instance", "dcmi").Push(); err != nil {
				log.Error("Could not push completion to Pushgateway:", config.Ipmi[index].Host, err)
				return
			} else {
				pushFlag = true
				log.Info("dcmi push success:", config.Ipmi[index].Host)
			}
		}
	}
	select {
	case <-ctx.Done():
		log.Error("收到超时信号，ipmi监控退出,", Config.Ipmi[index].Host)
		return
	default:
		if pushFlag {
			log.Info("ipmi goroutine监控中，", "设备:", config.Ipmi[index].Host)
		} else {
			log.Error("ipmi goroutine监控失败,设备", config.Ipmi[index].Host)
		}

	}
}

// Manage by daemon commands or run the daemon
func (service *Service) Manage() (string, error) {

	usage := "Usage: cron_job install | remove | start | stop | status"
	// If received any kind of command, do it
	if len(os.Args) > 1 {
		command := os.Args[1]
		switch command {
		case "install":
			return service.Install()
		case "remove":
			return service.Remove()
		case "start":
			return service.Start()
		case "stop":
			// No need to explicitly stop cron since job will be killed
			return service.Stop()
		case "status":
			return service.Status()
		default:
			return usage, nil
		}
	}
	// Set up channel on which to send signal notifications.
	// We must use a buffered channel or risk missing the signal
	// if we're not ready to receive when the signal is sent.
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, os.Kill, syscall.SIGTERM)

	// Create a new cron manager
	c := cron.New(cron.WithSeconds())
	// Run makefile every min
	c.AddFunc("*/"+strconv.Itoa(config.Global.Interval)+" * * * * *", func() {
		IPMIMonitor()
	})
	c.Start()
	select {}
	// Waiting for interrupt by system signal
	killSignal := <-interrupt
	log.Info("Got signal:", killSignal)
	return "Service exited", nil
}

func main() {
	srv, err := daemon.New(name, description, daemon.SystemDaemon)
	if err != nil {
		log.Error("Error: ", err)
		os.Exit(1)
	}
	service := &Service{srv}
	status, err := service.Manage()
	if err != nil {
		log.Error(status, "\nError: ", err)
		os.Exit(1)
	}
	fmt.Println(status)
}

// 单独的IPMI监控协程
func IPMIMonitor() {
	for i, _ := range config.Ipmi {
		go collectMonitoring(i, config)
	}
}

func parseSnmp(ip string, oid string, flag string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*time.Duration(config.Global.Wait))
	defer cancel()

	var pushFlag bool
	var snmpGauge *prometheus.GaugeVec
	pusher := push.New(config.Global.Pushgateway, config.Global.SNMPJob)

	session, err := wapsnmp.NewWapSNMP(ip, READ_COMM, wapsnmp.SNMPv2c, 2*time.Second, 1)
	defer session.Close()
	if err != nil {
		log.Error("SNMP_CONN_FAIL creating session => %v\n", err)
		snmpGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: flag,
			Help: "help..",
		}, []string{"Host", "Type","Oid"})
		snmpGauge.WithLabelValues( ip,"Err", oid).Set(0)
	} else {
		val, err := session.Get(wapsnmp.MustParseOid(oid))
		if err != nil {
			log.Error("SNMP_OID_FAIL getting => %v\n", err)
			snmpGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Name: flag,
				Help: "help..",
			}, []string{"Host", "Type","Oid"})
			snmpGauge.WithLabelValues( ip,"Err", oid).Set(0)
		} else {
			value := val.(float64)
			snmpGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Name: flag,
				Help: "help..",
			}, []string{ "Host", "Type", "Oid"})
			snmpGauge.WithLabelValues(ip, flag, oid).Set(value)
		}
	}
	pusher.Collector(snmpGauge)

	if err := pusher.Grouping("instance", "snmp").Push(); err != nil {
		log.Error("Could not push completion to Pushgateway:", ip, err)
		return
	} else {
		pushFlag = true
		log.Info("snmp push success:", ip)
	}

	select {
	case <-ctx.Done():
		log.Error("收到超时信号，snmp监控退出,", ip)
		return
	default:
		if pushFlag {
			log.Info("snmp goroutine监控中，", "设备:", ip)
		} else {
			log.Error("snmp goroutine监控失败,设备", ip)
		}
	}

}

func SNMPMonitor() {
	for _, lcp_ip := range snmp.Lcp_ips {
		for _, oid := range snmp.Lcp_oids {
			go parseSnmp(lcp_ip, oid, "LCP")
		}
	}
	for _, cool_ip := range snmp.Cool_ips {
		for _, cool_oid := range snmp.Cool_oids {
			go parseSnmp(cool_ip, cool_oid, "COOLOR")
		}
	}
	for _, pdu_ip := range snmp.Pdu_ips {
		for _, am_oid := range snmp.Pdu_am_oids {
			go parseSnmp(pdu_ip, am_oid, "PDU_AM")
		}
		for _, on_oid := range snmp.Pdu_on_oids {
			go parseSnmp(pdu_ip, on_oid, "PDU_ON")
		}
	}

}
