package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/PagerDuty/godspeed"
)

const (
	DefaultHost        = "127.0.0.1"
	DefaultPort        = 8125
	MaxBytes           = 8192
	StatsdRetrySeconds = 10
)

type StatsdEvents struct {
	Host string
	Port int
}

type StatsdMetrics struct {
	Host string
	Port int
}

func (e StatsdMetrics) Run(events chan *Metrics) {
	for {
		log.Print(fmt.Sprintf("attempting to connect to statsd: %s:%d", e.Host, e.Port))
		a, err := godspeed.New(e.Host, e.Port, false)
		if err == nil {
			defer a.Conn.Close()
			for event := range events {
				failed := false
				for _, m := range event.MetricsList {

					tags := []string{m.Tags}
					metric := fmt.Sprintf("%s.%s", m.Prefix, m.Name)

					err := a.Send(metric, m.Type, m.Value, 1, tags)
					if err != nil {
						log.Println(fmt.Sprintf("submitted a metric but it failed: %s", err))
						failed = true
						break
					}
				}
				if failed == true {
					break
				}
			}
		}
		msg := ""
		if err != nil {
			msg = fmt.Sprintf(": %s", err)
		}
		log.Print(fmt.Sprintf("can't connect to statsd to send metrics, trying again in %d seconds%s", StatsdRetrySeconds, msg))
		time.Sleep(StatsdRetrySeconds * time.Second)
	}
}

func (e StatsdEvents) Run(events chan *Events) {
	for {
		log.Print(fmt.Sprintf("attempting to connect to statsd: %s:%d", e.Host, e.Port))
		a, err := godspeed.New(e.Host, e.Port, false)
		if err == nil {
			defer a.Conn.Close()
			for event := range events {
				failed := false
				for _, e := range event.EventList {
					optionals := make(map[string]string)
					optionals["alert_type"] = e.Status
					optionals["source_type_name"] = "kubernetes"
					addlTags := []string{e.Tags}

					err := a.Event(e.Message, e.Raw, optionals, addlTags)
					if err != nil {
						log.Println(fmt.Sprintf("submited an event but it failed: %s", err))
						failed = true
						break
					}
				}
				if failed == true {
					break
				}
			}
		}
		msg := ""
		if err != nil {
			msg = fmt.Sprintf(": %s", err)
		}
		log.Print(fmt.Sprintf("can't connect to statsd to send events, trying again in %d seconds%s", StatsdRetrySeconds, msg))
		time.Sleep(StatsdRetrySeconds * time.Second)
	}
}

func loadStatsdHostPort() (string, int) {
	var host string
	var port int
	host = os.Getenv("STATSD_HOST")
	if host == "" {
		host = DefaultHost
	}
	port_checker := os.Getenv("STATSD_PORT")
	if port_checker == "" {
		port = DefaultPort
	} else {
		port_int, err := strconv.Atoi(port_checker)
		if err != nil {
			port = DefaultPort
		}
		port = port_int
	}
	return host, port
}

func LoadStatsdMetricsConfig() StatsdMetrics {
	host, port := loadStatsdHostPort()
	return StatsdMetrics{
		Host: host,
		Port: port,
	}
}

func LoadStatsdEventsConfig() StatsdEvents {
	host, port := loadStatsdHostPort()
	return StatsdEvents{
		Host: host,
		Port: port,
	}
}
