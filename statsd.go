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
	Tags string
}

type StatsdMetrics struct {
	Host string
	Port int
	Tags string
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

					tags := []string{m.Tags + "," + e.Tags}
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
				for _, event := range event.EventList {
					optionals := make(map[string]string)
					optionals["alert_type"] = event.Status
					optionals["source_type_name"] = "kubernetes"
					addlTags := []string{event.Tags + "," + e.Tags}

					err := a.Event(event.Message, event.Raw, optionals, addlTags)
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

func loadStatsdHostPort() (string, int, string) {
	var host string
	var port int
	var env_tags string
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
	env_tags = os.Getenv("TAGS")

	return host, port, env_tags
}

func LoadStatsdMetricsConfig() StatsdMetrics {
	host, port, tags := loadStatsdHostPort()
	return StatsdMetrics{
		Host: host,
		Port: port,
		Tags: tags,
	}
}

func LoadStatsdEventsConfig() StatsdEvents {
	host, port, tags := loadStatsdHostPort()
	return StatsdEvents{
		Host: host,
		Port: port,
		Tags: tags,
	}
}
