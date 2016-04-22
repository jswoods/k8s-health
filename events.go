package main

type Events struct {
	EventList []Event
}

type Event struct {
	Sources  string
	Tags     string
	Status   string
	Priority string
	Reason   string
	Message  string
	Raw      string
}
