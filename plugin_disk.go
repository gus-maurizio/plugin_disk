package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/shirou/gopsutil/disk"
	log "github.com/sirupsen/logrus"
    	"time"
)

var PluginEnv		[]disk.PartitionStat
var PluginConfig 	map[string]map[string]map[string]interface{}
var PluginData		map[string]interface{}


func PluginMeasure() ([]byte, []byte, float64) {
	// Get measurement of Disks
	for k := range PluginData {delete(PluginData, k)}
	d, _  := disk.Partitions(false)
	for _, part := range d {
		PluginData[part.Mountpoint], _ = disk.Usage(part.Mountpoint)
	}
	// Make it understandable
	// Apply USE methodology for DISK
	// U: 	Usage (usually throughput/latency indicators)
	//		In this case we define as CPU average utilization 0-100
	//		Latency is the MHz of each CPU weighted by usage
	// S:	Saturation (how full is the filesystem)
	// E:	Errors (not applicable for DISK)
	// Prepare the data

	myMeasure, _				:= json.Marshal(PluginData)
	myMeasureRaw, _ 			:= json.Marshal(PluginData)
	return myMeasure, myMeasureRaw, float64(time.Now().UnixNano())/1e9
}

func PluginAlert(measure []byte) (string, string, bool, error) {
	// log.WithFields(log.Fields{"MyMeasure": string(MyMeasure[:]), "measure": string(measure[:])}).Info("PluginAlert")
	// var m 			interface{}
	// err := json.Unmarshal(measure, &m)
	// if err != nil { return "unknown", "", true, err }
	alertMsg  := ""
	alertLvl  := ""
	alertFlag := false
	alertErr  := errors.New("no error")

	return alertMsg, alertLvl, alertFlag, alertErr
}


func InitPlugin(config string) () {
	if PluginData == nil {
		PluginData = make(map[string]interface{},20)
	}
	if PluginConfig == nil {
		PluginConfig = make(map[string]map[string]map[string]interface{},20)
	}
	PluginEnv, _	= disk.Partitions(false)
	err := json.Unmarshal([]byte(config), &PluginConfig)
	if err != nil {
		log.WithFields(log.Fields{"config": config}).Error("failed to unmarshal config")
	}
	log.WithFields(log.Fields{"pluginconfig": PluginConfig, "pluginenv": PluginEnv }).Info("InitPlugin")
}


func main() {
	// for testing purposes only, can safely not exist!
	// config := " { \"alert\": {    \"blue\":   [0,  3], \"green\":  [3,  60], \"yellow\": [60, 80], \"orange\": [80, 90], \"red\":    [90, 100] } } "
	config  := 	`
				{"alert": { 
				            "/": 		{"low": 10, "design": 60.0, "engineered": 80.0},
				            "/opt":		{"low": 05, "design": 40.0, "engineered": 70.0}
				          }
				}
				`
	InitPlugin(config)
	log.WithFields(log.Fields{"PluginConfig": PluginConfig}).Info("InitPlugin")
	tickd := 1* time.Second
	for i := 1; i <= 2; i++ {
		tick := time.Now().UnixNano()
		measure, measureraw, measuretimestamp := PluginMeasure()
		alertmsg, alertlvl, isAlert, err := PluginAlert(measure)
		fmt.Printf("Iteration #%d tick %d \n", i, tick)
		log.WithFields(log.Fields{"timestamp": measuretimestamp, 
					  "measure": string(measure[:]),
					  "measureraw": string(measureraw[:]),
					  "PluginData": PluginData,
					  "alertMsg": alertmsg,
					  "alertLvl": alertlvl,
					  "isAlert":  isAlert,
					  "AlertErr":      err,
		}).Info("Tick")
		time.Sleep(tickd)
	}
}
