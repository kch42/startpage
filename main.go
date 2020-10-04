package main

import (
	"errors"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
	"github.com/silvasur/startpage/weather"
)

var porn EarthPorn
var curWeather weather.Weather

func trylater(ch chan<- bool) {
	log.Println("Will try again later...")
	time.Sleep(1 * time.Minute)
	ch <- true
}

func earthPornUpdater(ch chan bool) {
	for _ = range ch {
		newporn, err := GetEarthPorn()
		if err != nil {
			log.Print(err)
			go trylater(ch)
			continue
		}

		porn = newporn
		log.Println("New fap material!")
	}
}

var place = ""

func setPlaceCmd(params []string) error {
	if len(params) != 1 {
		return errors.New("set-weather-place needs one parameter")
	}

	place = params[0]
	return nil
}

func weatherUpdater(ch chan bool) {
	for _ = range ch {
		newW, err := weather.CurrentWeather(place)
		if err != nil {
			log.Printf("Failed getting latest weather data: %s", err)
			go trylater(ch)
			continue
		}

		curWeather = newW
		log.Println("New weather data")
	}
}

func intervalUpdates(d time.Duration, stopch <-chan bool, chans ...chan<- bool) {
	send := func(chans ...chan<- bool) {
		for _, ch := range chans {
			go func(ch chan<- bool) {
				ch <- true
			}(ch)
		}
	}

	send(chans...)

	tick := time.NewTicker(d)
	for {
		select {
		case <-tick.C:
			send(chans...)
		case <-stopch:
			tick.Stop()
			for _, ch := range chans {
				close(ch)
			}
			return
		}
	}
}

var tpl *template.Template

func loadTemplate() {
	gopaths := strings.Split(os.Getenv("GOPATH"), ":")
	for _, p := range gopaths {
		var err error
		tpl, err = template.ParseFiles(path.Join(p, "src", "github.com", "silvasur", "startpage", "template.html"))
		if err == nil {
			return
		}
	}

	panic(errors.New("could not find template in $GOPATH/src/github.com/silvasur/startpage"))
}

func initCmds() {
	RegisterCommand("add-link", addLinkCmd)
	RegisterCommand("set-earthporn-savepath", setSavepathCmd)
	RegisterCommand("set-weather-place", setPlaceCmd)
	RegisterCommand("set-maxdim", setMaxdimCmd)
}

func runConf() {
	f, err := os.Open(os.ExpandEnv("$HOME/.startpagerc"))
	if err != nil {
		log.Fatalf("Could not open startpagerc: %s", err)
	}
	defer f.Close()

	if err := RunCommands(f); err != nil {
		log.Fatal(err)
	}
}

func main() {
	laddr := flag.String("laddr", ":25145", "Listen on this port")
	flag.Parse()

	loadTemplate()
	initCmds()
	runConf()

	pornch := make(chan bool)
	weatherch := make(chan bool)
	stopch := make(chan bool)

	go intervalUpdates(30*time.Minute, stopch, pornch, weatherch)
	go weatherUpdater(weatherch)
	go earthPornUpdater(pornch)

	defer func(stopch chan<- bool) {
		stopch <- true
	}(stopch)

	http.HandleFunc("/", startpage)
	http.HandleFunc("/bgimg", bgimg)
	http.HandleFunc("/savebg", savebg)
	log.Fatal(http.ListenAndServe(*laddr, nil))
}

type TplData struct {
	Porn    *EarthPorn
	Weather *weather.Weather
	Links   []Link
}

func startpage(rw http.ResponseWriter, req *http.Request) {
	defer req.Body.Close()

	if err := tpl.Execute(rw, &TplData{&porn, &curWeather, links}); err != nil {
		log.Printf("Failed executing template: %s\n", err)
	}
}

func bgimg(rw http.ResponseWriter, req *http.Request) {
	defer req.Body.Close()

	if len(porn.data) == 0 {
		rw.WriteHeader(http.StatusNotFound)
	}

	rw.Header().Add("Content-Type", porn.mediatype)
	if _, err := rw.Write(porn.data); err != nil {
		log.Printf("Failed serving background: %s", err)
	}
}

func savebg(rw http.ResponseWriter, req *http.Request) {
	defer req.Body.Close()

	if len(porn.data) == 0 {
		fmt.Fprintln(rw, "No earth porn available")
		return
	}

	if err := (&porn).save(); err != nil {
		log.Println(err)
		fmt.Fprintln(rw, err)
	}

	rw.Header().Add("Location", "/")
	rw.WriteHeader(http.StatusFound)
}
