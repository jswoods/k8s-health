package main

type EventReciever interface {
	Run(events chan *Events)
}

type MetricReciever interface {
	Run(metrics chan *Metrics)
}

func LoadEventRecievers() []EventReciever {
	plugins := make([]EventReciever, 1)
	plugins[0] = LoadStatsdEventsConfig()
	return plugins
}
func LoadMetricsRecievers() []MetricReciever {
	plugins := make([]MetricReciever, 1)
	plugins[0] = LoadStatsdMetricsConfig()
	return plugins
}
