package main

import (
	"Collect/utils/conf"
	"fmt"
	"github.com/cdevr/WapSNMP"
	log "github.com/cihub/seelog"
	"github.com/robfig/cron"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)
type SNMP struct {

}
type Community string
const (
	READ_COMM = "public"
	WRITE_COMM = "private"
)

type Monitor uint8
const (
	LCP Monitor = 0
	COOLOR Monitor = 1
	PDU_ON Monitor = 2
	PDU_AM Monitor = 3
)

type SOMEFAIL uint8
const (
	SNMP_CONN_FAIL = 1
	SNMP_OID_FAIL = 1
)

type SnmpHitback struct {  // each declare must Capp-title, json use it.
	TimeStamp  int64
	Ip string
	Mon Monitor
	//Values []int
	Values []interface{}
}

type Targets struct {
	Ip string
	Mon Monitor
	version wapsnmp.SNMPVersion
	s_ids *[]string
}

func getSnmp(ip string, oids *[]wapsnmp.Oid, version wapsnmp.SNMPVersion,ch chan(map[string]interface{})) {
	session, err := wapsnmp.NewWapSNMP(ip, READ_COMM, version, 2*time.Second, 1)
	if err != nil {
		log.Error("Error creating session => %v\n", err)
		log.Error(SNMP_CONN_FAIL, err)
	}

	defer session.Close()

	oidMap := make(map[string]interface{})
	for _, oid := range *oids {
		val, err := session.Get(oid)
		if err != nil {
			log.Error("Error getting => %v\n", err)
			log.Error(SNMP_OID_FAIL, err)
		}
		oidMap[oid.String()] = strconv.FormatInt(val.(int64), 10)
	}

	oidMap["time"] = time.Now().UnixNano()/1e6
	oidMap["host"] = ip
	oidMap["name"] = "snmp"
	ch <- oidMap
}


func work(ips []string, s_oids *[]string, version wapsnmp.SNMPVersion, m Monitor)[]map[string]interface{}{
	oids := make([]wapsnmp.Oid, len(*s_oids))
	snmpMap := []map[string]interface{}{}
	snmpMapChan := make(chan map[string]interface{})
	for i, v := range *s_oids {
		oids[i] = wapsnmp.MustParseOid(v)
	}
	for _,ip := range ips{
		go getSnmp(ip, &oids, version, snmpMapChan)
	}
	for i := 0; i < len(ips); i++{
		snmpMap = append(snmpMap, <-snmpMapChan)
	}
	return snmpMap
}

type snmp_monitor struct {
	Lcp_ips []string
	Lcp_oids []string

	Cool_ip []string
	Cool_oids []string

	Pdu_ips []string
	Pdu_on_oids []string
	Pdu_am_oids []string
}

func main() {
	cr := cron.New()
	spec := "*/10 * * * * ?"
	//k := 0
	var version = wapsnmp.SNMPv1
	cr.AddFunc(spec, func() {
		data, _ := ioutil.ReadFile("snmpFile.yml")
		s := snmp_monitor{}
		yaml.Unmarshal(data, &s)
		chs  := []map[string]interface{}{}
		if len(s.Lcp_ips) > 0 && len(s.Lcp_oids) > 0 {
			for _,ch := range work(s.Lcp_ips, &s.Lcp_oids, version, LCP){
				chs = append(chs, ch)
			}

		}
		if len(s.Cool_ip) > 0 && len(s.Cool_oids) > 0 {
			for _,ch := range work(s.Cool_ip, &s.Cool_oids, wapsnmp.SNMPv2c, COOLOR){
				chs = append(chs,ch)
			}
		}
		if len(s.Pdu_ips) > 0 {
			if len(s.Pdu_am_oids) > 0 {
				for _,ch := range work(s.Pdu_ips, &s.Pdu_am_oids, wapsnmp.SNMPv2c, PDU_AM) {
					chs = append(chs,ch)
				}
			}
			if len(s.Pdu_on_oids) > 0 {
				for _,ch := range work(s.Pdu_ips, &s.Pdu_on_oids, wapsnmp.SNMPv2c, PDU_ON) {
					chs = append(chs,ch)
				}
			}

		}
		for i,ch := range chs {
			fmt.Println(i,ch)
		}
	})
	cr.Start()
	select {}
}
var cronTime string
var filePath string
func init() {
	myConfig := new(conf.Config)
	dir := getCurrentDirectory()
	myConfig.InitConfig(dir + "/cf.Config")
	cronTime = myConfig.Read("snmp", "cronTime")
	filePath = myConfig.Read("snmp", "filePath")
}
//func Reader(rc chan map[string]interface{})  {
//	cr := cron.New()
//	spec := "*/" + cronTime + " * * * * ?"
//	var version = wapsnmp.SNMPv1
//	cr.AddFunc(spec, func() {
//		data, _ := ioutil.ReadFile(filePath)
//		s := snmp_monitor{}
//		yaml.Unmarshal(data, &s)
//		chs  := []map[string]interface{}{}
//		if len(s.Lcp_ips) > 0 && len(s.Lcp_oids) > 0 {
//			for _,ch := range work(s.Lcp_ips, &s.Lcp_oids, version, LCP){
//				chs = append(chs, ch)
//			}
//		}
//		if len(s.Cool_ip) > 0 && len(s.Cool_oids) > 0 {
//			for _,ch := range work(s.Cool_ip, &s.Cool_oids, wapsnmp.SNMPv2c, COOLOR){
//				chs = append(chs,ch)
//			}
//		}
//		if len(s.Pdu_ips) > 0 {
//			if len(s.Pdu_am_oids) > 0 {
//				for _,ch := range work(s.Pdu_ips, &s.Pdu_am_oids, wapsnmp.SNMPv2c, PDU_AM) {
//					chs = append(chs,ch)
//				}
//			}
//			if len(s.Pdu_on_oids) > 0 {
//				for _,ch := range work(s.Pdu_ips, &s.Pdu_on_oids, wapsnmp.SNMPv2c, PDU_ON) {
//					chs = append(chs,ch)
//				}
//			}
//
//		}
//		for _,ch := range chs {
//			rc <- ch
//		}
//	})
//	cr.Start()
//	select {}
//}
func getCurrentDirectory() string {
	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		panic(err)
	}
	return strings.Replace(dir, "\\", "/", -1)
}