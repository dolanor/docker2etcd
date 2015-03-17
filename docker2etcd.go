package main

import (
	"flag"
	"fmt"
	docker "github.com/fsouza/go-dockerclient"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	etcdEndpoint   string
	dockerEndpoint string
	watching       bool
	wg             sync.WaitGroup
)

func initFlags() {
	flag.StringVar(&etcdEndpoint, "etcdEndpoint", "", "etcd API endpoint")
	flag.StringVar(&dockerEndpoint, "dockerEndpoint", "", "docker API endpoint")
	flag.Parse()
}

func getEndpoint() (string, error) {
	defaultEndpoint := "unix:///var/run/docker.sock"
	if os.Getenv("DOCKER_HOST") != "" {
		defaultEndpoint = os.Getenv("DOCKER_HOST")
	}
	if dockerEndpoint != "" {
		defaultEndpoint = dockerEndpoint
	}
	_, _, err := parseHost(defaultEndpoint)
	if err != nil {
		return "", err
	}
	return defaultEndpoint, nil
}

func parseHost(addr string) (string, string, error) {
	var (
		proto string
		host  string
		port  int
	)
	addr = strings.TrimSpace(addr)
	switch {
	case addr == "tcp://":
		return "", "", fmt.Errorf("Invalid bind address format: %s", addr)
	case strings.HasPrefix(addr, "unix://"):
		proto = "unix"
		addr = strings.TrimPrefix(addr, "unix://")
		if addr == "" {
			addr = "/var/run/docker.sock"
		}
	case strings.HasPrefix(addr, "tcp://"):
		proto = "tcp"
		addr = strings.TrimPrefix(addr, "tcp://")
	case strings.HasPrefix(addr, "fd://"):
		return "fd", addr, nil
	case addr == "":
		proto = "unix"
		addr = "/var/run/docker.sock"
	default:
		if strings.Contains(addr, "://") {
			return "", "", fmt.Errorf("Invalid bind address protocol: %s", addr)
		}
		proto = "tcp"
	}
	if proto != "unix" && strings.Contains(addr, ":") {
		hostParts := strings.Split(addr, ":")
		if len(hostParts) != 2 {
			return "", "", fmt.Errorf("Invalid bind address format: %s", addr)
		}
		if hostParts[0] != "" {
			host = hostParts[0]
		} else {
			host = "127.0.0.1"
		}
		if p, err := strconv.Atoi(hostParts[1]); err == nil && p != 0 {
			port = p
		} else {
			return "", "", fmt.Errorf("Invalid bind address format: %s", addr)
		}
	} else if proto == "tcp" && !strings.Contains(addr, ":") {
		return "", "", fmt.Errorf("Invalid bind address format: %s", addr)
	} else {
		host = addr
	}
	if proto == "unix" {
		return proto, host, nil
	}
	return proto, fmt.Sprintf("%s:%d", host, port), nil
}

func main() {
	initFlags()
	fmt.Println(etcdEndpoint, dockerEndpoint)

	dendpoint, err := getEndpoint()
	if err != nil {
		log.Fatal("Endpoint not valid: ", err)
	}
	dclient, err := docker.NewClient(dendpoint) //NewDockerClient(dendpoint)
	if err != nil {
		log.Fatal("Unable to create docker client: ", err)
	}

	containers, err := dclient.ListContainers(docker.ListContainersOptions{All: true})
	if err != nil {
		log.Fatal("Couldn't list containers: ", err)
	}

	for _, i := range containers {

		fmt.Println(i.Names)
	}

	wg.Add(1)
	defer wg.Done()
	eventChan := make(chan *docker.APIEvents, 100)
	defer close(eventChan)
	for {
		watching = false

		if !watching {
			err = dclient.AddEventListener(eventChan)
			if err != nil && err != docker.ErrListenerAlreadyExists {
				log.Println("Error registering event listener: ", err)
				time.Sleep(10 * time.Second)
				continue
			}
			watching = true
		}
		select {
		case event := <-eventChan:
			if event == nil {
				if watching {
					dclient.RemoveEventListener(eventChan)
					watching = false
					dclient = nil
				}
				break
			}

			if event.Status == "start" || event.Status == "stop" || event.Status == "die" {
				log.Println("Received event ", event.Status, " for container ", event.ID[:12])
			}

		case <-time.After(10 * time.Second):

		}
	}

	wg.Wait()
}
