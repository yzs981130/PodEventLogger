package main

import (
	"encoding/json"
	"flag"
	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"log"
	"path/filepath"
	"sort"
	"time"
)

var clientset *kubernetes.Clientset


// lastLogSet contains every events which has been processed
var lastLogSet sets.String

var lastLogEvents []v1.Event

const lastLogEventsThresholdCnt = 1000
const lastLogEventsThresholdTime = 24 * time.Hour

// we only consider event which has an older timestamp than lastLogTimestamp each time
// for others should have been processed or it's too older to consider
var lastLogTimestamp time.Time

func buildConfig(master, kubeconfig string) (*rest.Config, error) {
	if master != "" || kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags(master, kubeconfig)
	}
	return rest.InClusterConfig()
}

func cleanup() {
	if len(lastLogEvents) > lastLogEventsThresholdCnt {
		pos := -1
		for i, m := range lastLogEvents {
			if m.LastTimestamp.After(time.Now().Add(-lastLogEventsThresholdTime)) {
				pos = i
				break
			}
		}
		if pos == -1 {
			log.Fatal("lastLogEventsThresholdCnt and lastLogEventsThresholdTime mismatch")
			return
		}
		lastLogEvents = lastLogEvents[pos+1:]
		lastLogSet = make(sets.String)
		for _, m := range lastLogEvents {
			lastLogSet.Insert(m.Name)
		}
	}
}

func work() {
	cleanup()

	var currEvents []v1.Event
	currLastLogTimestamp := lastLogTimestamp

	data, err := clientset.RESTClient().Get().AbsPath("api/v1/events").DoRaw()
	if err != nil {
		log.Fatal("can't get events: " + err.Error())
	}
	var events v1.EventList
	err = json.Unmarshal(data, &events)
	if err != nil {
		log.Fatal("error in parsing json: " + err.Error())
	}
	for _, event := range events.Items {
		// if current event is older than lastLogTimestamp, ignore
		if event.LastTimestamp.Time.Before(lastLogTimestamp) {
			continue
		}
		// if current event has `just` been processed, ignore
		// store it if is not older than lastLogTimestamp
		if event.LastTimestamp.Time.Equal(lastLogTimestamp) {
			if lastLogSet.Has(event.Name) {
				continue
			}
		}
		// process current event, sort by lastTimeStamp, log it
		currEvents = append(currEvents, event)
		lastLogSet.Insert(event.Name)
		// update lastLogTimestamp by currLastLogTimestamp
		if currLastLogTimestamp.Before(event.LastTimestamp.Time) {
			currLastLogTimestamp = event.LastTimestamp.Time
		}
	}
	// update lastLogTimestamp
	lastLogTimestamp = currLastLogTimestamp
	less := func(i, j int) bool {
		return currEvents[i].LastTimestamp.Time.Before(currEvents[j].LastTimestamp.Time)
	}
	sort.Slice(currEvents, less)
	lastLogEvents = append(lastLogEvents, currEvents...)
	for _, v := range currEvents {
		t, _ := json.Marshal(v)
		log.Print(string(t))
	}
}

func main() {
	var kubeconfig *string
	var logdir *string
	kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	logdir = flag.String("logdir", "/log", "absolute path to log dir")
	flag.Parse()

	log.SetFlags(log.Flags() &^ (log.Ldate | log.Ltime))
	logf, err := rotatelogs.New(
		filepath.Join(*logdir, "PodEvent_log.%Y%m%d%H%M"),
		rotatelogs.WithLinkName(filepath.Join(*logdir, "PodEvent_log")),
		rotatelogs.WithRotationTime(24*time.Hour))
	if err != nil {
		log.Printf("failed to create rotatelogs: %s", err)
		panic("can't write log to " + *logdir)
	}

	config, err := buildConfig("", *kubeconfig)
	if err != nil {
		log.Fatal(err.Error())
	}
	clientset, err = kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatal(err.Error())
	}

	log.Print("write log to " + *logdir)

	log.SetOutput(logf)

	lastLogSet = make(sets.String)
	lastLogTimestamp = time.Now().Add(-5 * 24 * time.Hour)

	wait.Forever(work, 30 * time.Second)
}
