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
var diskioIndicator = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "sreagent_disk_iop",
		Help: "Disk IO",
	}, []string{"disk","operation"} )

var diskbIndicator = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "sreagent_disk_bytes",
		Help: "Disk Bytes",
	}, []string{"disk","operation"} )


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
		diskioIndicator.WithLabelValues(diskid, "read").Add(float64(inc_riop))
		diskioIndicator.WithLabelValues(diskid, "write").Add(float64(inc_wiop))
		diskbIndicator.WithLabelValues(diskid,  "read").Add(float64(inc_rbytes))
		diskbIndicator.WithLabelValues(diskid, 	"write").Add(float64(inc_wbytes))

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
	}
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
	prometheus.MustRegister(diskioIndicator)
	prometheus.MustRegister(diskbIndicator)

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
		fmt.Printf("Iteration #%d tick %d %v\n", i, tick,PluginData["io"].(map[string]map[string]float64),
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
