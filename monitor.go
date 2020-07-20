package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"github.com/jinzhu/configor"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"
	"github.com/prometheus/common/log"
	"github.com/robfig/cron"
	"io/ioutil"
	"math"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Config struct {
	Global struct {
		Pushurl  string
		Job      string
		Interval int
		Wait     int
		Driver   string
		Type     []string
	}

	Ipmi []struct {
		Host string
		User string
		Pwd  string
	}
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

var (
	config                     = Config{}
	ipmiChassisPowerRegex      = regexp.MustCompile(`^System Power\s*:\s(?P<value>.*)`)
	ipmiChassisPowerFaultRegex = regexp.MustCompile(`^Power fault\s*:\s(?P<value>.*)`)
	ipmiChassisDriveFaultRegex = regexp.MustCompile(`^Power fault\s*:\s(?P<value>.*)`)
	ipmiChassisFanFaultRegex   = regexp.MustCompile(`^Cooling/fan fault\s*:\s(?P<value>.*)`)
)

func init() {
	configor.Load(&config, "./conf/config.yml")
}

func readFile(filename string) []byte {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Error("File reading error", err)
	}
	return data
}

func getValue(ipmiOutput []byte, regex *regexp.Regexp) (string, error) {
	for _, line := range strings.Split(string(ipmiOutput), "\n") {
		match := regex.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		for i, name := range regex.SubexpNames() {
			if name != "value" {
				continue
			}
			return match[i], nil
		}
	}
	return "", fmt.Errorf("Could not find value in output: %s", string(ipmiOutput))
}

func execute(cmdStr string, cmdArgs string) ([]byte, error) {
	cmd := exec.Command(cmdStr, cmdArgs)
	stdout, _ := cmd.StdoutPipe()
	defer stdout.Close()

	if err := cmd.Start(); err != nil {
		log.Error("cmd.Start: %v", err)
	}
	log.Info(cmd.Args) //print current cmd

	//cmdPid := &cmd.Process.Pid
	//log.Info("cmd pid:", cmdPid) //print cmd pid

	var res int
	if err := cmd.Wait(); err != nil {
		if ex, ok := err.(*exec.ExitError); ok {
			res = ex.Sys().(syscall.WaitStatus).ExitStatus() // get cmd result code
			log.Info("cmd exit status", res)
		}
	}
	return ioutil.ReadAll(stdout) //read stout result
}

func splitMonitoringOutput(impiOutput []byte) ([]sensorData, error) {
	var result []sensorData

	r := csv.NewReader(bytes.NewReader(impiOutput))
	fields, err := r.ReadAll()

	for _, line := range fields {
		line = strings.Fields(line[0])
		var data sensorData

		data.ID, err = strconv.ParseInt(line[0], 10, 64)
		if err != nil {
			continue
		}
		data.Name = line[2]
		data.Type = line[4]
		data.State = line[6]

		value := line[8]
		if value != "N/A" {
			data.Value, err = strconv.ParseFloat(value, 64)
			if err != nil {
				return result, err
			}
		} else {
			data.Value = math.NaN()
		}

		data.Unit = line[10]
		data.Event = strings.Trim(line[12], "'")

		result = append(result, data)
	}
	return result, err
}

func getChassisPowerState(ipmiOutput []byte, substr string, regex *regexp.Regexp) (float64, error) {
	value, err := getValue(ipmiOutput, regex)
	if err != nil {
		log.Error(err)
		return -1, err
	}

	if strings.Contains(value, substr) {
		return 1, err
	}
	return 0, err
}

//ipmimonitoring -D LAN_2_0 -h remote_ip -u username -p password
//ipmi-chassis -D LAN_2_0 -h remote_ip -u username -p password --get-status

func collectMonitoring(index int, Config Config) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*time.Duration(config.Global.Wait))
	defer cancel()

	pusher := push.New(Config.Global.Pushurl, Config.Global.Job)
	var ipmiGauge *prometheus.GaugeVec
	var ipmiGaugeState *prometheus.GaugeVec
	var ChassSystemPowerGaugeState *prometheus.GaugeVec
	var ChassisPowerFaultGauge *prometheus.GaugeVec
	var ChassisFanFaultGauge *prometheus.GaugeVec
	var ChassisDriveFaultGauge *prometheus.GaugeVec

	for _, Mclass := range Config.Global.Type {
		switch Mclass {
		case "ipmimonitoring":
			output, err := execute("ipmimonitoring", "-D "+Config.Global.Driver+
				" -h "+Config.Ipmi[index].Host+" -u "+Config.Ipmi[index].User+" -p "+Config.Ipmi[index].Pwd)
			if err != nil {
				log.Error(err)
			}
			//output := readFile("sugonIPMI.txt")
			results, err := splitMonitoringOutput(output)
			if err != nil {
				log.Error(err.Error())
			}

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
					panic(data.State)
				}

				ipmiGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
					Name: data.Name,
					Help: "help..",
				}, []string{"Name", "Host", "Event", "Stata", "Type", "Unit", "Id"})
				ipmiGaugeState = prometheus.NewGaugeVec(prometheus.GaugeOpts{
					Name: data.Name + "_state",
					Help: "help..",
				}, []string{"Name", "Host", "Event", "Stata", "Type", "Unit", "Id"})
				ipmiGauge.WithLabelValues(data.Name, config.Ipmi[index].Host, data.Event, data.State, data.Type, data.Unit, strconv.FormatInt(data.ID, 10)).Set(data.Value)
				ipmiGaugeState.WithLabelValues(data.Name, config.Ipmi[index].Host, data.Event, data.State, data.Type, data.Unit, strconv.FormatInt(data.ID, 10)).Set(state)
				pusher.Collector(ipmiGauge)
				pusher.Collector(ipmiGaugeState)
			}
			//instanceName := "ipmi_" + config.Ipmi[index].Host
			//fmt.Println("instanceName:",instanceName)
			//if err := pusher.Grouping("instance",instanceName).Push(); err != nil {
			//	//log.Error("Could not push completion time to Pushgateway:", err)
			//	log.Error(config.Ipmi[index].Host,err)
			//} else {
			//	log.Info("push success",config.Ipmi[index].Host)
			//}

		case "ipmi-chassis":
			//		//	output,err := execute("ipmi-chassis","-D " + Config.Global.Driver +
			//		//		" -h " + Config.Ipmi[index].Host + " -u " + Config.Ipmi[index].User + " -p "+ Config.Ipmi[index].Pwd + " --get-status")
			//		//	if err != nil {
			//		//		panic(err)
			//		//	}
			output := readFile("sugonClass.txt")
			ChassisSystemPower, _ := getChassisPowerState(output, "on", ipmiChassisPowerRegex)
			ChassisPowerFault, _ := getChassisPowerState(output, "false", ipmiChassisPowerFaultRegex)
			ChassisFanFault, _ := getChassisPowerState(output, "false", ipmiChassisFanFaultRegex)
			ChassisDriveFault, _ := getChassisPowerState(output, "false", ipmiChassisDriveFaultRegex)
			//fmt.Println(ChassisSystemPower,ChassisPowerFault,ChassisFanFault,ChassisDriveFault)

			ChassSystemPowerGaugeState = prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Name: "ChassisSystemPowerState",
				Help: "help..",
			}, []string{"host"})
			ChassSystemPowerGaugeState.WithLabelValues(Config.Ipmi[index].Host).Set(ChassisSystemPower)

			ChassisPowerFaultGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Name: "ChassisPowerFaultState",
				Help: "help..",
			}, []string{"host"})
			ChassisPowerFaultGauge.WithLabelValues(Config.Ipmi[index].Host).Set(ChassisPowerFault)
			//pusher

			ChassisFanFaultGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Name: "ChassisPowerFaultState",
				Help: "help..",
			}, []string{"host"})
			ChassisFanFaultGauge.WithLabelValues(Config.Ipmi[index].Host).Set(ChassisFanFault)
			//pusher

			ChassisDriveFaultGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Name: "ChassisPowerFaultState",
				Help: "help..",
			}, []string{"host"})
			ChassisDriveFaultGauge.WithLabelValues(Config.Ipmi[index].Host).Set(ChassisDriveFault)
			//pusher.Collector(ChassSystemPowerGaugeState).Collector(ChassisPowerFaultGauge).Collector(ChassisFanFaultGauge).Collector(ChassisDriveFaultGauge)

		}
		instanceName := "ipmi_" + config.Ipmi[index].Host
		if err := pusher.Grouping("instance", instanceName).Push(); err != nil {
			//log.Error("Could not push completion time to Pushgateway:", err)
			log.Error(config.Ipmi[index].Host, err)
		} else {
			log.Info("push success:", config.Ipmi[index].Host)
		}

	}

	select {
	case <-ctx.Done():
		log.Error("收到信号，监控退出,time=", time.Now().Unix(), Config.Ipmi[index].Host)
		return
	default:
		log.Info("goroutine监控中,time=", time.Now().Unix(), "设备:", config.Ipmi[index].Host)
	}

}

func main() {
	//pusher := push.New("http://192.168.37.128:9091/","ipmi_monitor")

	cr := cron.New()
	spec := "*/" + strconv.Itoa(config.Global.Interval) + " * * * * ?"
	cr.AddFunc(spec, func() {
		fmt.Println("开始定时任务")
		go monitor(config)
	})
	cr.Start()
	select {}
}

// 单独的监控协程
func monitor(config Config) {
	for i, _ := range config.Ipmi {
		go collectMonitoring(i, config)
	}
}
