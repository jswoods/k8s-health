package main

import (
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"strconv"
	"time"
)

const (
	defaultCollectMetrics = true
	defaultCollectEvents  = true
)

var (
	collectMetrics bool
	collectEvents  bool
)

func handler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "All good yo")
}

func main() {
	log.Println("Starting")
	collectMetricsStr := os.Getenv("COLLECT_METRICS")
	collectEventsStr := os.Getenv("COLLECT_EVENTS")

	if b, err := strconv.ParseBool(collectMetricsStr); err == nil {
		collectMetrics = b
	} else {
		collectMetrics = defaultCollectMetrics
	}

	if b, err := strconv.ParseBool(collectEventsStr); err == nil {
		collectEvents = b
	} else {
		collectEvents = defaultCollectEvents
	}

	events_collector_chan := make(chan *Events)
	metrics_collector_chan := make(chan *Metrics)

	events_reciever_chans := make(map[EventReciever]chan *Events)
	metrics_reciever_chans := make(map[MetricReciever]chan *Metrics)

	events_recievers := LoadEventRecievers()
	metrics_recievers := LoadMetricsRecievers()

	for _, v := range events_recievers {
		events_reciever_chans[v] = make(chan *Events)
	}
	for _, v := range metrics_recievers {
		metrics_reciever_chans[v] = make(chan *Metrics)
	}

	if collectEvents {
		log.Println("Starting event collection")
		event_collectors := LoadEventCollectors()

		for _, v := range event_collectors {
			if v != nil {
				go EventProducer(v, events_collector_chan)
			}
		}

		for v, c := range events_reciever_chans {
			go v.Run(c)
		}
	}

	if collectMetrics {
		log.Println("Starting metric collection")
		metric_collectors := LoadMetricCollectors()

		for _, v := range metric_collectors {
			if v != nil {
				go MetricProducer(v, metrics_collector_chan)
			}
		}

		for v, c := range metrics_reciever_chans {
			go v.Run(c)
		}
	}

	go func() {
		http.HandleFunc("/", handler)
		log.Println(http.ListenAndServe(":8080", nil))
	}()

	for {
		select {
		case e := <-events_collector_chan:
			for _, re := range events_reciever_chans {
				re <- e
			}
		case m := <-metrics_collector_chan:
			for _, rm := range metrics_reciever_chans {
				rm <- m
			}
		default:
			// Do this sleep to cut back on cpu usage
			time.Sleep(1 * time.Millisecond)
		}
	}
}
