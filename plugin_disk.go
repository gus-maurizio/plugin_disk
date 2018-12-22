package main

import (
		"encoding/json"
		"errors"
		"fmt"
		"github.com/shirou/gopsutil/disk"
		log "github.com/sirupsen/logrus"
		"github.com/prometheus/client_golang/prometheus"
		"github.com/prometheus/client_golang/prometheus/promhttp"
		"net/http"		
		"time"
)

//	Define the metrics we wish to expose
var diskIndicator = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "sreagent_disk",
		Help: "Disk Stats",
	}, []string{"disk","measure","operation"} )

//	Define the metrics we wish to expose
var diskRates = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "sreagent_disk_rates",
		Help: "disk IO Throughput",
	}, []string{"disk", "unit", "operation"} )


var PluginConfig 	map[string]map[string]map[string]interface{}
var PluginData 		map[string]interface{}



func PluginMeasure() ([]byte, []byte, float64) {
	// Get measurement of IOCounters
	PluginData["ts_current"]		= time.Now().UnixNano()
	PluginData["io_current"], _ 	= disk.IOCounters()
	
	Δts := PluginData["ts_current"].(int64) - PluginData["ts_previous"].(int64)		// nanoseconds!
	
	DISKS:
	for diskid, _ := range PluginData["io_current"].(map[string]disk.IOCountersStat) {
		_, present := PluginData["io_previous"].(map[string]disk.IOCountersStat)[diskid]
		if !present {continue DISKS}
		inc_riop 	:= PluginData["io_current"].(map[string]disk.IOCountersStat)[diskid].ReadCount  - PluginData["io_previous"].(map[string]disk.IOCountersStat)[diskid].ReadCount
		inc_wiop 	:= PluginData["io_current"].(map[string]disk.IOCountersStat)[diskid].WriteCount - PluginData["io_previous"].(map[string]disk.IOCountersStat)[diskid].WriteCount
		inc_rbytes 	:= PluginData["io_current"].(map[string]disk.IOCountersStat)[diskid].ReadBytes  - PluginData["io_previous"].(map[string]disk.IOCountersStat)[diskid].ReadBytes
		inc_wbytes 	:= PluginData["io_current"].(map[string]disk.IOCountersStat)[diskid].WriteBytes - PluginData["io_previous"].(map[string]disk.IOCountersStat)[diskid].WriteBytes

		// Update metrics related to the plugin
		diskIndicator.WithLabelValues(diskid, "io_operations", "read").Add(float64(inc_riop))
		diskIndicator.WithLabelValues(diskid, "io_operations", "write").Add(float64(inc_wiop))
		diskIndicator.WithLabelValues(diskid, "io_bytes",      "read").Add(float64(inc_rbytes))
		diskIndicator.WithLabelValues(diskid, "io_bytes",      "write").Add(float64(inc_wbytes))

		PluginData["io"] = map[string]map[string]float64	{
			diskid: {
				"riops":		float64(inc_riop)/float64(Δts) * 1e9,
				"wiops":		float64(inc_wiop)/float64(Δts) * 1e9,
				},
		}

		PluginData["mbps"] = map[string]map[string]float64	{
			diskid: {
				"rmbps":		float64(inc_rbytes)/float64(Δts) * 1e3,
				"wmbps":		float64(inc_wbytes)/float64(Δts) * 1e3,
			},
		}
		diskRates.WithLabelValues(diskid, "iops",  "read"   ).Set(float64(inc_riop)/float64(Δts) * 1e9)
		diskRates.WithLabelValues(diskid, "iops",  "write"  ).Set(float64(inc_wiop)/float64(Δts) * 1e9)
		diskRates.WithLabelValues(diskid, "mbps",  "read"   ).Set(float64(inc_rbytes)/float64(Δts) * 1e3)
		diskRates.WithLabelValues(diskid, "mbps",  "write"  ).Set(float64(inc_wbytes)/float64(Δts) * 1e3)
	}
	PluginData["ts_previous"] = PluginData["ts_current"]
	PluginData["io_previous"] = PluginData["io_current"]
	myMeasure, _ := json.Marshal(PluginData)
	return myMeasure, []byte(""), float64(PluginData["ts_current"].(int64)) / 1e9
}

func PluginAlert(measure []byte) (string, string, bool, error) {
	// log.WithFields(log.Fields{"MyMeasure": string(MyMeasure[:]), "measure": string(measure[:])}).Info("PluginAlert")
	// var m 			interface{}
	// err := json.Unmarshal(measure, &m)
	// if err != nil { return "unknown", "", true, err }
	alertMsg := ""
	alertLvl := ""
	alertFlag := false
	alertErr := errors.New("no error")

	return alertMsg, alertLvl, alertFlag, alertErr
}

func InitPlugin(config string) {
	if PluginData == nil {
		PluginData = make(map[string]interface{}, 20)
	}
	if PluginConfig == nil {
		PluginConfig = make(map[string]map[string]map[string]interface{}, 20)
	}
	err := json.Unmarshal([]byte(config), &PluginConfig)
	if err != nil {
		log.WithFields(log.Fields{"config": config}).Error("failed to unmarshal config")
	}
	PluginData["ts_previous"]	 	= time.Now().UnixNano()
	PluginData["io_previous"], _ 	= disk.IOCounters()
	// Register metrics with prometheus
	prometheus.MustRegister(diskIndicator)
	prometheus.MustRegister(diskRates)

	log.WithFields(log.Fields{"pluginconfig": PluginConfig, "plugindata": PluginData}).Info("InitPlugin")
}

func main() {
	config  := 	`
				{
					"alert": 
					{
						"cpu":
						{
							"low": 			2,
							"design": 		60.0,
							"engineered":	80.0
						}
				    }
				}
				`

	//--------------------------------------------------------------------------//
	// time to start a prometheus metrics server
	// and export any metrics on the /metrics endpoint.
	http.Handle("/metrics", promhttp.Handler())
	go func() {
		http.ListenAndServe(":8999", nil)
	}()
	//--------------------------------------------------------------------------//

	InitPlugin(config)
	log.WithFields(log.Fields{"PluginConfig": PluginConfig}).Info("InitPlugin")
	tickd := 1 * time.Second
	for i := 1; i <= 100; i++ {
		tick := time.Now().UnixNano()
		measure, measureraw, measuretimestamp := PluginMeasure()
		alertmsg, alertlvl, isAlert, err := PluginAlert(measure)
		fmt.Printf("Iteration #%d tick %d %v %v\n", i, tick,PluginData["io"].(map[string]map[string]float64),
														 PluginData["mbps"].(map[string]map[string]float64))
		log.WithFields(log.Fields{"timestamp": measuretimestamp,
			"measure":    string(measure[:]),
			"measureraw": string(measureraw[:]),
			"PluginData": PluginData,
			"alertMsg":   alertmsg,
			"alertLvl":   alertlvl,
			"isAlert":    isAlert,
			"AlertErr":   err,
		}).Debug("Tick")
		time.Sleep(tickd)
	}
}
