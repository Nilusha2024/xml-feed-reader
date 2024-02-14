package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/clbanning/mxj"
)

type event struct {
	path string
	op   string
}

func main() {
	// parse configuration
	var cfgPath string
	flag.StringVar(&cfgPath, "c", "./config.json", "config file path")
	flag.Parse()
	flag.PrintDefaults()
	cfg, err := parseConfig(cfgPath)
	if err != nil {
		log.Fatal("issues detected in configuration file", err.Error())
	}

	// queue holds un-processed filepaths
	queue := make(chan event)
	readErrors := make(chan string)

	// can use multiple readers as preferred
	for i := 0; i < 8; i++ {
		go worker(queue, readErrors)
	}

	prevState := make(map[string]os.FileInfo)
	for {
		select {
		case <-time.After(time.Millisecond * 100):
			currentState := make(map[string]os.FileInfo)
			// iterate through feed locations
			for _, feedLocation := range cfg.Feeds {
				files, err := ioutil.ReadDir(feedLocation)
				if err != nil {
					continue
				}
				for _, currentInfo := range files {
					if path, err := filepath.Abs(fmt.Sprintf("%v%c%v", feedLocation, os.PathSeparator, currentInfo.Name())); err == nil {
						previousInfo, exists := prevState[path]
						currentState[path] = currentInfo
						if !exists {
							queue <- event{path: path, op: "CREATE"}
						} else if previousInfo.ModTime() != currentInfo.ModTime() {
							queue <- event{path: path, op: "MODIFIED"}
						}
					}
				}
			}
			if len(currentState) > 0 {
				prevState = currentState
			}
		case errPath, _ := <-readErrors:
			delete(prevState, errPath)
		}
	}
}

func worker(queue chan event, readErrors chan string) {
	for {
		select {
		case evt, ok := <-queue:
			if !ok {
				return
			}
			fmt.Println(evt.op, evt.path)
			data, err := readFile(evt.path)
			if err != nil {
				readErrors <- evt.path
			}

			// sends data to server
			sendDataToServer(data)
		}
	}
}

// reads xml and parse into map
func readFile(path string) (data mxj.Map, err error) {

	xmlFile, err := os.Open(path)
	if err != nil {
		return
	}
	defer xmlFile.Close()
	data, err = mxj.NewMapXmlReader(xmlFile)
	return
}

// sends data to server
func sendDataToServer(data mxj.Map) {
	payload, err := json.Marshal(data)
	if err != nil {
		log.Fatal(err)
	}
	resp, err := http.Post("http://172.16.10.49:4000/data", "application/json", bytes.NewBuffer(payload))
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	var result bool
	json.NewDecoder(resp.Body).Decode(&result)
	log.Println(result)
}

type config struct {
	Feeds []string `json:"feeds,omitempty"`
}

func parseConfig(path string) (cfg config, err error) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()
	decorder := json.NewDecoder(file)
	err = decorder.Decode(&cfg)
	return
}
