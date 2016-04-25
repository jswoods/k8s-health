package main

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	kube_api "k8s.io/kubernetes/pkg/api"
	client_extensions "k8s.io/kubernetes/pkg/apis/extensions"
	restclient "k8s.io/kubernetes/pkg/client/restclient"
	client "k8s.io/kubernetes/pkg/client/unversioned"
	kube_fields "k8s.io/kubernetes/pkg/fields"
	kube_labels "k8s.io/kubernetes/pkg/labels"
)

const (
	KubeRetrySeconds = 10
)

type Credentials struct {
	Username string
	Password string
}

type KubeEvents struct {
	Url         string
	Credentials Credentials
	Version     string
	Path        string
	Interval    time.Duration
}

type KubeEventParsed struct {
	Type   string
	Object kube_api.Event `json:","object"`
}

type KubeMetricDetailConfig struct {
	Url         string
	Credentials Credentials
	Version     string
	Path        string
	Interval    time.Duration
}

type KubeMetricConfig struct {
	Url         string
	Credentials Credentials
	Version     string
	Path        string
	Interval    time.Duration
}

type Http struct {
	Url         string
	Path        string
	Credentials Credentials
}

func (h Http) Request(verb string) (*http.Response, error) {
	log.Printf(fmt.Sprintf("polling %s", h.Url))
	url := fmt.Sprintf("%s%s", h.Url, h.Path)
	caCertPool := x509.NewCertPool()
	tr := &http.Transport{
		TLSClientConfig:    &tls.Config{RootCAs: caCertPool, InsecureSkipVerify: true},
		DisableCompression: true,
	}
	client := &http.Client{
		Transport: tr,
	}
	req, _ := http.NewRequest(verb, url, nil)
	req.SetBasicAuth(h.Credentials.Username, h.Credentials.Password)
	return client.Do(req)
}

func (e KubeEvents) Run(events chan *Events) {
	httpReq := &Http{
		Url:         e.Url,
		Path:        e.Path,
		Credentials: e.Credentials,
	}
	response, err := httpReq.Request("GET")
	if err != nil {
		return
	}
	defer response.Body.Close()
	log.Printf(fmt.Sprintf("got response from %s", e.Url))
	log.Printf(fmt.Sprintf("now watching endpoint %s", e.Path))
	reader := bufio.NewReader(response.Body)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return
		}
		line = bytes.TrimSpace(line)
		jsonString := string(line[:])

		var kubeEventParsed KubeEventParsed

		err = json.Unmarshal([]byte(jsonString), &kubeEventParsed)
		if err != nil {
			log.Println(fmt.Sprintf("bad event: %s", jsonString))
			return
		}

		kubeEvent := kubeEventParsed.Object

		status := "info"
		priority := "low"

		if kubeEvent.Reason == "Unhealthy" {
			status = "error"
			priority = "high"
		}

		events <- &Events{
			EventList: []Event{
				Event{
					Sources:  fmt.Sprintf("kubernetes,%s", kubeEvent.Source),
					Tags:     fmt.Sprintf("source_type:kubernetes,namespace:%s,%s:%s", kubeEvent.Namespace, strings.ToLower(kubeEvent.Kind), kubeEvent.Name),
					Status:   status,
					Priority: priority,
					Reason:   kubeEvent.Reason,
					Message:  fmt.Sprintf("%s. This event has occurred %d times", kubeEvent.Message, kubeEvent.Count),
					Raw:      fmt.Sprintf("%s", kubeEvent),
				},
			},
		}
	}
	fmt.Println("Failed to read response ... retrying in 5 seconds...")
	time.Sleep(5 * time.Second)
}

func getDeploymentList(client *client.ExtensionsClient) (deployments_list *client_extensions.DeploymentList, err error) {
	options := kube_api.ListOptions{}
	deployments_interface := client.Deployments(kube_api.NamespaceAll)
	return deployments_interface.List(options)
}

func (e KubeMetricDetailConfig) Run(metrics chan *Metrics) {
	metricsGroup := &Metrics{}
	config := &restclient.Config{
		Host:     e.Url,
		Username: e.Credentials.Username,
		Password: e.Credentials.Password,
		Insecure: true,
	}

	log.Printf(fmt.Sprintf("polling %s", e.Url))
	client, err := client.New(config)
	if err != nil {
		log.Printf(fmt.Sprintf("failed to connect to %s! trying again in %d seconds...", e.Url, KubeRetrySeconds))
		time.Sleep(KubeRetrySeconds * time.Second)
		return
	}

	option_set := make(map[string]string)
	options := kube_api.ListOptions{LabelSelector: kube_labels.SelectorFromSet(option_set), FieldSelector: kube_fields.SelectorFromSet(option_set)}

	deployments, err := getDeploymentList(client.ExtensionsClient)
	if err != nil {
		log.Printf(fmt.Sprintf("failed to list deployments from url %s %s! trying again in %d seconds...", deployments, e.Url, KubeRetrySeconds))
		log.Printf(fmt.Sprintf("%s", err))
		time.Sleep(KubeRetrySeconds * time.Second)
		return
	}
	for _, deployment := range deployments.Items {
		pod_interface := client.Pods(deployment.Namespace)
		options := kube_api.ListOptions{LabelSelector: kube_labels.SelectorFromSet(deployment.Spec.Selector.MatchLabels), FieldSelector: kube_fields.SelectorFromSet(option_set)}
		pods, err := pod_interface.List(options)
		if err != nil {
			log.Printf(fmt.Sprintf("failed list pods for deployment %s!", deployment.Name))
			continue
		}
		running, waiting, succeeded, failed := 0, 0, 0, 0
		for _, pod := range pods.Items {
			for _, containerStatus := range pod.Status.ContainerStatuses {
				var state string
				waiting := containerStatus.State.Waiting
				running := containerStatus.State.Running
				terminated := containerStatus.State.Terminated

				if terminated != nil {
					state = "terminated"
				}
				if waiting != nil {
					state = "waiting"
				}
				if running != nil {
					state = "running"
				}
				metric_name := fmt.Sprintf("container_status.%s", state)
				containerStatuses := Metric{
					Prefix: "kubernetes",
					Name:   metric_name,
					Type:   "g",
					Value:  float64(1),
					Tags:   fmt.Sprintf("source_type:kubernetes,pod:%s,container:%s,namespace:%s,ready:%s", containerStatus.Name, pod.Name, pod.Namespace, strconv.FormatBool(containerStatus.Ready)),
				}
				metricsGroup.MetricsList = append(metricsGroup.MetricsList, containerStatuses)
			}
			switch pod.Status.Phase {
			case kube_api.PodRunning:
				running++
			case kube_api.PodPending:
				waiting++
			case kube_api.PodSucceeded:
				succeeded++
			case kube_api.PodFailed:
				failed++
			}
		}
		replicaSetPercentRunning := Metric{
			Prefix: "kubernetes",
			Name:   "replica_set.percent_running",
			Type:   "g",
			Value:  float64(float64(running) / float64(deployment.Status.Replicas) * 100),
			Tags:   fmt.Sprintf("source_type:kubernetes,replica_set:%s,namespace:%s", deployment.Name, deployment.Namespace),
		}
		replicaSetPercentWaiting := Metric{
			Prefix: "kubernetes",
			Name:   "replica_set.percent_waiting",
			Type:   "g",
			Value:  float64(float64(waiting) / float64(deployment.Status.Replicas) * 100),
			Tags:   fmt.Sprintf("source_type:kubernetes,replica_set:%s,namespace:%s", deployment.Name, deployment.Namespace),
		}
		replicaSetPercentSucceeded := Metric{
			Prefix: "kubernetes",
			Name:   "replica_set.percent_succeeded",
			Type:   "g",
			Value:  float64(float64(succeeded) / float64(deployment.Status.Replicas) * 100),
			Tags:   fmt.Sprintf("source_type:kubernetes,replica_set:%s,namespace:%s", deployment.Name, deployment.Namespace),
		}
		replicaSetPercentFailed := Metric{
			Prefix: "kubernetes",
			Name:   "replica_set.percent_failed",
			Type:   "g",
			Value:  float64(float64(failed) / float64(deployment.Status.Replicas) * 100),
			Tags:   fmt.Sprintf("source_type:kubernetes,replica_set:%s,namespace:%s", deployment.Name, deployment.Namespace),
		}
		deploymentSpecs := Metric{
			Prefix: "kubernetes",
			Name:   "deployment.replicas.spec",
			Type:   "g",
			Value:  float64(deployment.Spec.Replicas),
			Tags:   fmt.Sprintf("source_type:kubernetes,deployment:%s,namespace:%s", deployment.Name, deployment.Namespace),
		}
		deploymentStatuses := Metric{
			Prefix: "kubernetes",
			Name:   "deployment.replicas.status",
			Type:   "g",
			Value:  float64(deployment.Status.Replicas),
			Tags:   fmt.Sprintf("source_type:kubernetes,deployment:%s,namespace:%s", deployment.Name, deployment.Namespace),
		}
		deploymentUpdatedReplicas := Metric{
			Prefix: "kubernetes",
			Name:   "deployment.replicas.updated",
			Type:   "g",
			Value:  float64(deployment.Status.UpdatedReplicas),
			Tags:   fmt.Sprintf("source_type:kubernetes,deployment:%s,namespace:%s", deployment.Name, deployment.Namespace),
		}
		deploymentAvailableReplicas := Metric{
			Prefix: "kubernetes",
			Name:   "deployment.replicas.available",
			Type:   "g",
			Value:  float64(deployment.Status.AvailableReplicas),
			Tags:   fmt.Sprintf("source_type:kubernetes,deployment:%s,namespace:%s", deployment.Name, deployment.Namespace),
		}
		deploymentUnavailableReplicas := Metric{
			Prefix: "kubernetes",
			Name:   "deployment.replicas.unavailable",
			Type:   "g",
			Value:  float64(deployment.Status.UnavailableReplicas),
			Tags:   fmt.Sprintf("source_type:kubernetes,deployment:%s,namespace:%s", deployment.Name, deployment.Namespace),
		}
		metricsGroup.MetricsList = append(metricsGroup.MetricsList, deploymentSpecs)
		metricsGroup.MetricsList = append(metricsGroup.MetricsList, deploymentStatuses)
		metricsGroup.MetricsList = append(metricsGroup.MetricsList, deploymentUpdatedReplicas)
		metricsGroup.MetricsList = append(metricsGroup.MetricsList, deploymentAvailableReplicas)
		metricsGroup.MetricsList = append(metricsGroup.MetricsList, deploymentUnavailableReplicas)
		metricsGroup.MetricsList = append(metricsGroup.MetricsList, replicaSetPercentRunning)
		metricsGroup.MetricsList = append(metricsGroup.MetricsList, replicaSetPercentWaiting)
		metricsGroup.MetricsList = append(metricsGroup.MetricsList, replicaSetPercentSucceeded)
		metricsGroup.MetricsList = append(metricsGroup.MetricsList, replicaSetPercentFailed)
	}

	rc_interface := client.ReplicationControllers(kube_api.NamespaceAll)
	rcs, err := rc_interface.List(options)
	if err != nil {
		log.Printf(fmt.Sprintf("failed to list replication controllers from url %s! trying again in %d seconds...", e.Url, KubeRetrySeconds))
		log.Printf(fmt.Sprintf("%s", err))
		time.Sleep(KubeRetrySeconds * time.Second)
		return
	}
	for _, rc := range rcs.Items {
		pod_interface := client.Pods(rc.Namespace)
		options := kube_api.ListOptions{LabelSelector: kube_labels.SelectorFromSet(rc.Spec.Selector), FieldSelector: kube_fields.SelectorFromSet(option_set)}
		pods, err := pod_interface.List(options)
		if err != nil {
			log.Printf(fmt.Sprintf("failed list pods for rc %s!", rc.Name))
			continue
		}
		running, waiting, succeeded, failed := 0, 0, 0, 0
		for _, pod := range pods.Items {
			for _, containerStatus := range pod.Status.ContainerStatuses {
				var state string
				waiting := containerStatus.State.Waiting
				running := containerStatus.State.Running
				terminated := containerStatus.State.Terminated

				if terminated != nil {
					state = "terminated"
				}
				if waiting != nil {
					state = "waiting"
				}
				if running != nil {
					state = "running"
				}
				metric_name := fmt.Sprintf("container_status.%s", state)
				containerStatuses := Metric{
					Prefix: "kubernetes",
					Name:   metric_name,
					Type:   "g",
					Value:  float64(1),
					Tags:   fmt.Sprintf("source_type:kubernetes,pod:%s,container:%s,namespace:%s,ready:%s", containerStatus.Name, pod.Name, pod.Namespace, strconv.FormatBool(containerStatus.Ready)),
				}
				containerRestarts := Metric{
					Prefix: "kubernetes",
					Name:   "container.restarts",
					Type:   "g",
					Value:  float64(containerStatus.RestartCount),
					Tags:   fmt.Sprintf("source_type:kubernetes,pod:%s,container:%s,namespace:%s", containerStatus.Name, pod.Name, pod.Namespace),
				}
				metricsGroup.MetricsList = append(metricsGroup.MetricsList, containerStatuses)
				metricsGroup.MetricsList = append(metricsGroup.MetricsList, containerRestarts)
			}
			switch pod.Status.Phase {
			case kube_api.PodRunning:
				running++
			case kube_api.PodPending:
				waiting++
			case kube_api.PodSucceeded:
				succeeded++
			case kube_api.PodFailed:
				failed++
			}
		}
		replicationControllerPercentRunning := Metric{
			Prefix: "kubernetes",
			Name:   "replication_controller.percent_running",
			Type:   "g",
			Value:  float64(float64(running) / float64(rc.Status.Replicas) * 100),
			Tags:   fmt.Sprintf("source_type:kubernetes,replication_controller:%s,namespace:%s", rc.Name, rc.Namespace),
		}
		replicationControllerPercentWaiting := Metric{
			Prefix: "kubernetes",
			Name:   "replication_controller.percent_waiting",
			Type:   "g",
			Value:  float64(float64(waiting) / float64(rc.Status.Replicas) * 100),
			Tags:   fmt.Sprintf("source_type:kubernetes,replication_controller:%s,namespace:%s", rc.Name, rc.Namespace),
		}
		replicationControllerPercentSucceeded := Metric{
			Prefix: "kubernetes",
			Name:   "replication_controller.percent_succeeded",
			Type:   "g",
			Value:  float64(float64(succeeded) / float64(rc.Status.Replicas) * 100),
			Tags:   fmt.Sprintf("source_type:kubernetes,replication_controller:%s,namespace:%s", rc.Name, rc.Namespace),
		}
		replicationControllerPercentFailed := Metric{
			Prefix: "kubernetes",
			Name:   "replication_controller.percent_failed",
			Type:   "g",
			Value:  float64(float64(failed) / float64(rc.Status.Replicas) * 100),
			Tags:   fmt.Sprintf("source_type:kubernetes,replication_controller:%s,namespace:%s", rc.Name, rc.Namespace),
		}
		metricsGroup.MetricsList = append(metricsGroup.MetricsList, replicationControllerPercentRunning)
		metricsGroup.MetricsList = append(metricsGroup.MetricsList, replicationControllerPercentWaiting)
		metricsGroup.MetricsList = append(metricsGroup.MetricsList, replicationControllerPercentSucceeded)
		metricsGroup.MetricsList = append(metricsGroup.MetricsList, replicationControllerPercentFailed)
	}
	metrics <- metricsGroup
	metricsGroup.MetricsList = []Metric{}
	time.Sleep(e.Interval * time.Millisecond)
}

type MetricParser struct {
	Regex *regexp.Regexp
}

func (p MetricParser) Run(line []byte) (tag_map string, metric int, metric_name string, err error) {
	line = bytes.TrimSpace(line)
	bodyLine := string(line[:])
	if string(bodyLine[0]) != "#" {
		result_slice := p.Regex.FindStringSubmatch(bodyLine)

		if len(result_slice) < 3 {
			err = errors.New("Not enough results")
			return tag_map, metric, metric_name, err
		}

		tag_map_tmp := strings.Replace(result_slice[2], "=", ":", -1)
		tag_map = strings.Replace(tag_map_tmp, "\"", "", -1)
		metric, _ = strconv.Atoi(result_slice[3])
		metric_name = result_slice[1]
	}
	return tag_map, metric, metric_name, nil
}

//func (e KubeMetricConfig) Run(metrics chan *Metrics) {
//	//re := regexp.MustCompile(`^(.+){(.+)}\s([0-9]+)$`)
//	//metric_parser := &MetricParser{Regex: re}
//	//encoder := &kube_runtime.Encoder{}
//	//decoder := &kube_runtime.Decoder{}
//	//codec := kube_runtime.NewCodec(encoder, decoder)
//	codec := kube_runtime.Serializer{}
//	group_version := &kube_api_unversioned.GroupVersion{
//		Group:   "api",
//		Version: "v1",
//	}
//	content_config := restclient.ContentConfig{
//		GroupVersion: group_version,
//		Codec:        kube_runtime.Codec{},
//	}
//	config := &restclient.Config{
//		Host:          e.Url,
//		APIPath:       e.Version,
//		Username:      e.Credentials.Username,
//		Password:      e.Credentials.Password,
//		ContentConfig: content_config,
//		//Codec:           codec,
//		TLSClientConfig: restclient.TLSClientConfig{},
//		Insecure:        true,
//	}
//	client, err := restclient.RESTClientFor(config)
//	if err != nil {
//		log.Printf(fmt.Sprintf("failed to get rest client: %s! trying again in %d seconds...", err, KubeRetrySeconds))
//		time.Sleep(KubeRetrySeconds * time.Second)
//		return
//	}
//	//request := restclient.NewRequest(client, "GET", fmt.Sprintf("%s%s", e.Url, e.Path), e.Version, restclient.ContentConfig{}, restclient.BackoffManager{}, util.RateLimiter{})
//	request := client.Get()
//	response := request.Do()
//	fmt.Println(response)
//
//	//httpReq := &Http{
//	//	Url:         e.Url,
//	//	Path:        e.Path,
//	//	Credentials: e.Credentials,
//	//}
//
//	//response, err := httpReq.Request("GET")
//	//if err != nil {
//	//	return
//	//}
//	//if err != nil {
//	//	log.Printf(fmt.Sprintf("failed to connect to %s! trying again in %d seconds...", e.Url, KubeRetrySeconds))
//	//	time.Sleep(KubeRetrySeconds * time.Second)
//	//	return
//	//}
//	//defer response.Body.Close()
//	//reader := bufio.NewReader(response.Body)
//	//for {
//	//	line, err := reader.ReadBytes('\n')
//	//	if err != nil {
//	//		break
//	//	}
//	//	tag_map, metric, metric_name, err := metric_parser.Run(line)
//	//	if err != nil {
//	//		continue
//	//	}
//	//	metrics <- &Metrics{
//	//		MetricsList: []Metric{
//	//			Metric{
//	//				Prefix: "kubernetes",
//	//				Name:   metric_name,
//	//				Type:   "c",
//	//				Value:  float64(metric),
//	//				Tags:   fmt.Sprintf("source_type:kubernetes,%s", tag_map),
//	//			},
//	//		},
//	//	}
//	//}
//	time.Sleep(e.Interval * time.Millisecond)
//}

func LoadKubeEventsConfig() KubeEvents {
	url := os.Getenv("API_SERVER_URL")
	if url == "" {
		log.Fatal("Please specify an env var API_SERVER_URL that contains the url name of the kubernetes API.")
	}
	username := os.Getenv("API_USERNAME")
	if username == "" {
		log.Fatal("Please specify an env var API_USERNAME that contains the username of the kubernetes API.")
	}
	password := os.Getenv("API_PASSWORD")
	if password == "" {
		log.Fatal("Please specify an env var API_PASSWORD that contains the password of the kubernetes API.")
	}
	return KubeEvents{
		Url: url,
		Credentials: Credentials{
			Username: username,
			Password: password,
		},
		Version:  "v1",
		Path:     "/api/v1/events?watch=true",
		Interval: 5000,
	}
}

func LoadKubeMetricsConfigList() MetricCollectors {
	url := os.Getenv("API_SERVER_URL")
	if url == "" {
		log.Fatal("Please specify an env var API_SERVER_URL that contains the url name of the kubernetes API.")
	}
	username := os.Getenv("API_USERNAME")
	if username == "" {
		log.Fatal("Please specify an env var API_USERNAME that contains the username of the kubernetes API.")
	}
	password := os.Getenv("API_PASSWORD")
	if password == "" {
		log.Fatal("Please specify an env var API_PASSWORD that contains the password of the kubernetes API.")
	}
	credentials := Credentials{
		Username: username,
		Password: password,
	}
	return MetricCollectors{
		Collectors: []MetricCollector{
			//KubeMetricConfig{
			//	Url:         url,
			//	Credentials: credentials,
			//	Version:     "v1",
			//	Path:        "/metrics",
			//	Interval:    10000,
			//},
			KubeMetricDetailConfig{
				Url:         url,
				Credentials: credentials,
				Version:     "v1",
				Path:        "",
				Interval:    15000,
			},
		},
	}
}
