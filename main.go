package main

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io/ioutil"
	"math"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

type sensorData struct {
	ID    int64
	Name  string
	Type  string
	State string
	Value float64
	Unit  string
	Event string
}

func init() {
	fmt.Println("this is init func")
}

func readFile(filename string) []byte {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		fmt.Println("File reading error", err)
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

func execute() {
	cmd := exec.Command("", "", "")
	stdout, _ := cmd.StdoutPipe()
	defer stdout.Close()

	if err := cmd.Start(); err != nil {
		panic("cmd.Start: %v")
	}
	fmt.Println(cmd.Args) //print current cmd

	cmdPid := &cmd.Process.Pid
	fmt.Println(cmdPid) //print cmd pid

	result, _ := ioutil.ReadAll(stdout) //read stout result
	resdata := string(result)
	fmt.Println(resdata) //print stdout string

	var res int
	if err := cmd.Wait(); err != nil {
		if ex, ok := err.(*exec.ExitError); ok {
			fmt.Println("cmd exit status")
			res = ex.Sys().(syscall.WaitStatus).ExitStatus() // get cmd result code
		}
	}
	fmt.Println(res)
}

func contains(s []int64, elm int64) bool {
	for _, a := range s {
		if a == elm {
			return true
		}
	}
	return false
}

func splitMonitoringOutput(impiOutput []byte) ([]sensorData, error) {
	// excludeSensorIds []int64
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
		////if contains(excludeSensorIds, data.ID) {
		////	continue
		////}
		//
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

func collectMonitoring() {
	output := readFile("sugonIPMI.txt")
	results, err := splitMonitoringOutput(output)
	if err != nil {
		panic(err)
	}
	for _, data := range results {
		//var state float64
		//
		//switch data.State {
		//case "Nominal":
		//	state = 0
		//case "Warning":
		//	state = 1
		//case "Critical":
		//	state = 2
		//case "N/A":
		//	state = math.NaN()
		//default:
		//	panic(data.State)
		//	state = math.NaN()
		//}
		fmt.Println(data)

		//switch data.Unit {
		//case "RPM":
		//	collectTypedSensor(ch, fanSpeedDesc, fanSpeedStateDesc, state, data)
		//case "C":
		//	collectTypedSensor(ch, temperatureDesc, temperatureStateDesc, state, data)
		//case "A":
		//	collectTypedSensor(ch, currentDesc, currentStateDesc, state, data)
		//case "V":
		//	collectTypedSensor(ch, voltageDesc, voltageStateDesc, state, data)
		//case "W":
		//	collectTypedSensor(ch, powerDesc, powerStateDesc, state, data)
		//default:
		//	collectGenericSensor(ch, state, data)
		//}
	}
}

func main() {
	fmt.Println("this is main func")
	collectMonitoring()
}
