package main

type Metrics struct {
	MetricsList []Metric
}

type Metric struct {
	Prefix string
	Name   string
	Type   string
	Value  float64
	Tags   string
}
