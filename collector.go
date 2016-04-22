package main

var (
	EventPlugins []EventCollector
)

type MetricCollectors struct {
	Collectors []MetricCollector
}

type EventCollector interface {
	Run(events chan *Events)
}

type MetricCollector interface {
	Run(metrics chan *Metrics)
}

func LoadMetricCollectors() []MetricCollector {
	plugins := make([]MetricCollector, 0)
	plugins = append(plugins, LoadKubeMetricsConfigList().Collectors...)
	plugins = append(plugins, LoadSelfCheckConfig().Collectors...)
	return plugins
}

func LoadEventCollectors() []EventCollector {
	plugins := make([]EventCollector, 0)
	plugins = append(plugins, LoadKubeEventsConfig())
	return plugins
}
