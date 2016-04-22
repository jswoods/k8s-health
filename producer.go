package main

func MetricProducer(c MetricCollector, metrics chan *Metrics) {
	for {
		c.Run(metrics)
	}
}

func EventProducer(c EventCollector, events chan *Events) {
	for {
		c.Run(events)
	}
}
