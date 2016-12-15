package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"os"

	kube_api "k8s.io/kubernetes/pkg/api"
	client_extensions "k8s.io/kubernetes/pkg/apis/extensions"
	rest_client "k8s.io/kubernetes/pkg/client/restclient"
	client "k8s.io/kubernetes/pkg/client/unversioned"
	kube_fields "k8s.io/kubernetes/pkg/fields"
	kube_labels "k8s.io/kubernetes/pkg/labels"
)

const (
	KubeRetrySeconds = 10
)

type KubeEvents struct {
	Version  string
	Path     string
	Interval time.Duration
}

type KubeEventParsed struct {
	Type   string
	Object kube_api.Event `json:","object"`
}

type KubeMetricDetailConfig struct {
	Version  string
	Path     string
	Interval time.Duration
}

type KubeMetricConfig struct {
	Version  string
	Path     string
	Interval time.Duration
}

func GetHttpClient() (*http.Client, string, error) {
	c, err := InClusterConfig()
	if err != nil {
		return nil, "", err
	}
	tr, err := rest_client.TransportFor(c)
	if err != nil {
		return nil, "", err
	}
	httpClient := &http.Client{Transport: tr}
	return httpClient, c.Host, nil
}

func (e KubeEvents) Run(events chan *Events) {
	httpClient, host, err := GetHttpClient()
	if err != nil {
		log.Printf(fmt.Sprintf("failed to connect to %s! with error %v trying again in %d seconds...", host, err, KubeRetrySeconds))
		time.Sleep(KubeRetrySeconds * time.Second)
		return
	}

	for {
		response, err := httpClient.Get(fmt.Sprintf("%s/%s", host, e.Path))
		log.Printf(fmt.Sprintf("got response from %s", host))
		if err != nil {
			continue
		}
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
	}
	fmt.Println("Failed to read response ... retrying in 5 seconds...")
	time.Sleep(5 * time.Second)
}

func getDeploymentList(client *client.ExtensionsClient) (deployments_list *client_extensions.DeploymentList, err error) {
	options := kube_api.ListOptions{}
	deployments_interface := client.Deployments(kube_api.NamespaceAll)
	return deployments_interface.List(options)
}

func InClusterConfig() (*rest_client.Config, error) {
	log.Printf("Figuring out connection information...")

	// check if use https
	if _, err := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount/" + kube_api.ServiceAccountRootCAKey); err == nil {
		log.Printf(fmt.Sprintf("Cluster connection will be using https"))
	}
	master_url := os.Getenv("API_SERVER_URL")
	if len(master_url) == 0 {
		log.Printf("API_SERVER_URL env varialble is not specified, will tru to use http://kubernetes")
		master_url = "http://kubernetes"
	}

	config := &rest_client.Config{
		Host: master_url,
	}

	token, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
	if err != nil {
		log.Fatal(err)
	}
	config.BearerToken = string(token)

	config.QPS = 20
	qps_s := os.Getenv("QPS")
	if qps_s != "" {
		qps, err := strconv.ParseFloat(qps_s, 32)
		if err == nil {
			config.QPS = float32(qps)
		}
	}

	config.Burst = 30
	burst_s := os.Getenv("BURST")
	if burst_s != "" {
		burst, err := strconv.ParseInt(burst_s, 10, 32)
		if err == nil {
			config.Burst = int(burst)
		}
	}

	return config, nil
}

func (e KubeMetricDetailConfig) Run(metrics chan *Metrics) {

	localConfig, err := InClusterConfig()

	client, err := client.New(localConfig)

	if err != nil {
		log.Printf(fmt.Sprintf("failed to connect to %s! trying again in %d seconds...", localConfig.Host, KubeRetrySeconds))
		time.Sleep(KubeRetrySeconds * time.Second)
		return
	}

	metricsGroup := &Metrics{}

	log.Printf(fmt.Sprintf("polling %s", localConfig.Host))

	option_set := make(map[string]string)
	options := kube_api.ListOptions{LabelSelector: kube_labels.SelectorFromSet(option_set), FieldSelector: kube_fields.SelectorFromSet(option_set)}

	deployments, err := getDeploymentList(client.ExtensionsClient)
	if err != nil {
		log.Printf(fmt.Sprintf("failed to list deployments from url %s %s! trying again in %d seconds...", deployments, localConfig.Host, KubeRetrySeconds))
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
					Tags:   fmt.Sprintf("source_type:kubernetes,pod:%s,container:%s,namespace:%s,ready:%s,%s", containerStatus.Name, pod.Name, pod.Namespace, strconv.FormatBool(containerStatus.Ready), getLabelTags(pod.ObjectMeta)),
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

		replicaSetTags := fmt.Sprintf("source_type:kubernetes,replica_set:%s,namespace:%s,%s", deployment.Name, deployment.Namespace, getLabelTags(deployment.ObjectMeta))
		deploymentTags := fmt.Sprintf("source_type:kubernetes,deployment:%s,namespace:%s,%s", deployment.Name, deployment.Namespace, getLabelTags(deployment.ObjectMeta))

		replicaSetPercentRunning := Metric{
			Prefix: "kubernetes",
			Name:   "replica_set.percent_running",
			Type:   "g",
			Value:  float64(float64(running) / float64(deployment.Status.Replicas) * 100),
			Tags:   replicaSetTags,
		}
		replicaSetPercentWaiting := Metric{
			Prefix: "kubernetes",
			Name:   "replica_set.percent_waiting",
			Type:   "g",
			Value:  float64(float64(waiting) / float64(deployment.Status.Replicas) * 100),
			Tags:   replicaSetTags,
		}
		replicaSetPercentSucceeded := Metric{
			Prefix: "kubernetes",
			Name:   "replica_set.percent_succeeded",
			Type:   "g",
			Value:  float64(float64(succeeded) / float64(deployment.Status.Replicas) * 100),
			Tags:   replicaSetTags,
		}
		replicaSetPercentFailed := Metric{
			Prefix: "kubernetes",
			Name:   "replica_set.percent_failed",
			Type:   "g",
			Value:  float64(float64(failed) / float64(deployment.Status.Replicas) * 100),
			Tags:   replicaSetTags,
		}
		deploymentSpecs := Metric{
			Prefix: "kubernetes",
			Name:   "deployment.replicas.spec",
			Type:   "g",
			Value:  float64(deployment.Spec.Replicas),
			Tags:   deploymentTags,
		}
		deploymentStatuses := Metric{
			Prefix: "kubernetes",
			Name:   "deployment.replicas.status",
			Type:   "g",
			Value:  float64(deployment.Status.Replicas),
			Tags:   deploymentTags,
		}
		deploymentUpdatedReplicas := Metric{
			Prefix: "kubernetes",
			Name:   "deployment.replicas.updated",
			Type:   "g",
			Value:  float64(deployment.Status.UpdatedReplicas),
			Tags:   deploymentTags,
		}
		deploymentAvailableReplicas := Metric{
			Prefix: "kubernetes",
			Name:   "deployment.replicas.available",
			Type:   "g",
			Value:  float64(deployment.Status.AvailableReplicas),
			Tags:   deploymentTags,
		}
		deploymentUnavailableReplicas := Metric{
			Prefix: "kubernetes",
			Name:   "deployment.replicas.unavailable",
			Type:   "g",
			Value:  float64(deployment.Status.UnavailableReplicas),
			Tags:   deploymentTags,
		}
		deploymentPercentAvailable := Metric{
			Prefix: "kubernetes",
			Name:   "deployment.percent_available",
			Type:   "g",
			Value:  float64(float64(deployment.Status.AvailableReplicas) / float64(deployment.Status.Replicas) * 100),
			Tags:   deploymentTags,
		}
		deploymentPercentUnavailable := Metric{
			Prefix: "kubernetes",
			Name:   "deployment.percent_unavailable",
			Type:   "g",
			Value:  float64(float64(deployment.Status.UnavailableReplicas) / float64(deployment.Status.Replicas) * 100),
			Tags:   deploymentTags,
		}
		metricsGroup.MetricsList = append(metricsGroup.MetricsList, deploymentSpecs)
		metricsGroup.MetricsList = append(metricsGroup.MetricsList, deploymentStatuses)
		metricsGroup.MetricsList = append(metricsGroup.MetricsList, deploymentUpdatedReplicas)
		metricsGroup.MetricsList = append(metricsGroup.MetricsList, deploymentAvailableReplicas)
		metricsGroup.MetricsList = append(metricsGroup.MetricsList, deploymentPercentAvailable)
		metricsGroup.MetricsList = append(metricsGroup.MetricsList, deploymentUnavailableReplicas)
		metricsGroup.MetricsList = append(metricsGroup.MetricsList, deploymentPercentUnavailable)
		metricsGroup.MetricsList = append(metricsGroup.MetricsList, replicaSetPercentRunning)
		metricsGroup.MetricsList = append(metricsGroup.MetricsList, replicaSetPercentWaiting)
		metricsGroup.MetricsList = append(metricsGroup.MetricsList, replicaSetPercentSucceeded)
		metricsGroup.MetricsList = append(metricsGroup.MetricsList, replicaSetPercentFailed)
	}

	rc_interface := client.ReplicationControllers(kube_api.NamespaceAll)
	rcs, err := rc_interface.List(options)
	if err != nil {
		log.Printf(fmt.Sprintf("failed to list replication controllers from url %s! trying again in %d seconds...", localConfig.Host, KubeRetrySeconds))
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
					Tags:   fmt.Sprintf("source_type:kubernetes,pod:%s,container:%s,namespace:%s,ready:%s,%s", containerStatus.Name, pod.Name, pod.Namespace, strconv.FormatBool(containerStatus.Ready), getLabelTags(pod.ObjectMeta)),
				}
				containerRestarts := Metric{
					Prefix: "kubernetes",
					Name:   "container.restarts",
					Type:   "g",
					Value:  float64(containerStatus.RestartCount),
					Tags:   fmt.Sprintf("source_type:kubernetes,pod:%s,container:%s,namespace:%s,%s", containerStatus.Name, pod.Name, pod.Namespace, getLabelTags(pod.ObjectMeta)),
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
			Tags:   fmt.Sprintf("source_type:kubernetes,replication_controller:%s,namespace:%s,%s", rc.Name, rc.Namespace, getLabelTags(rc.ObjectMeta)),
		}
		replicationControllerPercentWaiting := Metric{
			Prefix: "kubernetes",
			Name:   "replication_controller.percent_waiting",
			Type:   "g",
			Value:  float64(float64(waiting) / float64(rc.Status.Replicas) * 100),
			Tags:   fmt.Sprintf("source_type:kubernetes,replication_controller:%s,namespace:%s,%s", rc.Name, rc.Namespace, getLabelTags(rc.ObjectMeta)),
		}
		replicationControllerPercentSucceeded := Metric{
			Prefix: "kubernetes",
			Name:   "replication_controller.percent_succeeded",
			Type:   "g",
			Value:  float64(float64(succeeded) / float64(rc.Status.Replicas) * 100),
			Tags:   fmt.Sprintf("source_type:kubernetes,replication_controller:%s,namespace:%s,%s", rc.Name, rc.Namespace, getLabelTags(rc.ObjectMeta)),
		}
		replicationControllerPercentFailed := Metric{
			Prefix: "kubernetes",
			Name:   "replication_controller.percent_failed",
			Type:   "g",
			Value:  float64(float64(failed) / float64(rc.Status.Replicas) * 100),
			Tags:   fmt.Sprintf("source_type:kubernetes,replication_controller:%s,namespace:%s,%s", rc.Name, rc.Namespace, getLabelTags(rc.ObjectMeta)),
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

func getLabelTags(metadata kube_api.ObjectMeta) string {
	tags := ""
	for label, value := range metadata.Labels {
		tags = tags + fmt.Sprintf("%s:%s,", label, value)
	}
	return tags
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

func LoadKubeEventsConfig() KubeEvents {
	return KubeEvents{
		Version:  "v1",
		Path:     "/api/v1/events?watch=true",
		Interval: 5000,
	}
}

func LoadKubeMetricsConfigList() MetricCollectors {
	return MetricCollectors{
		Collectors: []MetricCollector{
			KubeMetricDetailConfig{
				Version:  "v1",
				Path:     "",
				Interval: 15000,
			},
		},
	}
}
