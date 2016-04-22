package main

import "time"

const (
	DefaultInterval = time.Duration(5000)
	DefaultName     = "self.check"
)

type SelfCheckConfig struct {
	Interval time.Duration
	Name     string
}

func (s SelfCheckConfig) Run(metrics chan *Metrics) {
	ticker := time.NewTicker(time.Millisecond * s.Interval)
	for range ticker.C {
		metrics <- &Metrics{
			MetricsList: []Metric{
				Metric{
					Prefix: "kubernetes",
					Name:   s.Name,
					Type:   "c",
					Value:  float64(1),
					Tags:   "source_type:kubernetes",
				},
			},
		}
	}
}

func LoadSelfCheckConfig() MetricCollectors {
	return MetricCollectors{
		Collectors: []MetricCollector{
			SelfCheckConfig{
				Interval: DefaultInterval,
				Name:     DefaultName,
			},
		},
	}
}
