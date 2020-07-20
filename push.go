package main

import (
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"
)

var (
	completionTime = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "completTime",
		Help: "完成时间",
	}, []string{"one", "two", "three", "instance"})
)

func main() {
	registry := prometheus.NewRegistry()
	completionTime.WithLabelValues("1", "2", "3", "instance").Set(12.3)
	registry.MustRegister(completionTime)
	completionTime := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "db_backup_last_completion_timestamp_seconds",
		Help: "The timestamp of the last successful completion of a DB backup.",
	})

	completionTime.SetToCurrentTime()
	ipmis := make([]string, 5)
	ipmis = append(ipmis, "root")
	ipmis = append(ipmis, "root1")
	ipmis = append(ipmis, "root2")
	ipmis = append(ipmis, "root3")
	ipmis = append(ipmis, "root4")
	ipmis = append(ipmis, "root5")
	for _, ipmi := range ipmis {
		if err := push.New("http://192.168.37.128:9091", "db").
			Collector(completionTime).Gatherer(registry).
			Grouping("db", ipmi).
			Push(); err != nil {
			fmt.Println("Could not push completion time to Pushgateway:", err)
		}
	}

}
