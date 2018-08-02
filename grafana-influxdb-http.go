package main

import (
	"fmt"
	"github.com/coreos/go-systemd/daemon"
	"github.com/influxdata/influxdb/client/v2"
	"github.com/pelletier/go-toml"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"
	
)

const (
	configFile = "/etc/grafana/influxdb/http/config.toml"
)

func aerr(err error){
	if err != nil {
        fmt.Println("Error:", err.Error())
        os.Exit(1)
    }
}

func berr(err error){
	if err != nil {
        log.Fatal(err)
    }
}

func slashSplitter(c rune) bool {
    return c == '/'
}

func influxDBClient(url string, username string, password string) client.Client {
	c, err := client.NewHTTPClient(client.HTTPConfig{
		Addr:     url,
		Username: username,
		Password: password,
	})
	aerr(err)
	return c
}

func createMetrics(probeName string, c client.Client, database string, measurement string, url string, code int, bytes int, elapsed float64){
	bp, err := client.NewBatchPoints(client.BatchPointsConfig{
		Database:  database,
		Precision: "s",
	})
	berr(err)
	
	eventTime := time.Now().Add(time.Second * -20)
	
	tags := map[string]string{
		"probe": probeName,
		"url": url,
	}
	fields := map[string]interface{}{
		"code":    code,
		"bytes":   bytes,
		"elapsed": elapsed,
	}
	point, err := client.NewPoint(
		measurement,
		tags,
		fields,
		eventTime.Add(time.Second*10),
	)
	if err != nil {
		log.Fatalln("Error: ", err)
	}

	bp.AddPoint(point)
	err = c.Write(bp)
	if err != nil {
		log.Fatal(err)
	}
	
	log.Printf("URL : %s, code : %s, bytes : %s, elapsed : %s", url, code, bytes, elapsed)
	daemon.SdNotify(false, "READY=1")
}

func main(){
	config, err := toml.LoadFile(configFile)
	aerr(err)
	
	probeName    := config.Get("influxdb.probe_name").(string)
	address      := config.Get("influxdb.address").(string)
	database     := config.Get("influxdb.database").(string)
	measurement  := config.Get("influxdb.measurement").(string)
	username     := config.Get("influxdb.username").(string)
	password     := config.Get("influxdb.password").(string)
	interval     := config.Get("options.interval").(string)
	interval2, _ := time.ParseDuration(interval)
	hosts	     := config.Get("hosts.hosts").([]interface{})

	log.Printf("Try to connect at %s with %s/%s", address, username, password)
	c := influxDBClient(address, username, password)

	for {
		for _, v := range hosts {
			url, _ := v.(string)
			go func(url string){
				start := time.Now()
				response, err := http.Get(url)
				berr(err)
				contents, err := ioutil.ReadAll(response.Body)
				defer response.Body.Close()
				berr(err)
				elapsed := time.Since(start).Seconds()
				code    := response.StatusCode
				bytes   := len(contents)
				log.Printf("URL : %s, code : %s, bytes : %s, elapsed : %s", url, code, bytes, elapsed)
				createMetrics(probeName, c, database, measurement, url, code, bytes, elapsed)
			}(url)
		}
		time.Sleep(interval2)
	}
}