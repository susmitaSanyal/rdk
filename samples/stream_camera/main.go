package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"go.viam.com/robotcore/rimage"

	"github.com/pion/mediadevices/pkg/driver"

	"github.com/edaniels/golog"
	"github.com/edaniels/gostream"
	"github.com/edaniels/gostream/codec/x264"
)

func main() {

	dump := flag.Bool("dump", false, "dump all camera info")

	flag.Parse()

	if *dump {
		all := driver.GetManager().Query(driver.FilterVideoRecorder())
		for _, d := range all {
			golog.Global.Debugf("%T %s", d, d.ID())
			golog.Global.Debugf("\t label: %s", d.Info().Label)
			golog.Global.Debugf("\t props: %s", d.Properties())

		}
		return
	}

	port := 5555
	if flag.NArg() >= 1 {
		portParsed, err := strconv.ParseInt(flag.Arg(1), 10, 32)
		if err != nil {
			golog.Global.Fatal(err)
		}
		port = int(portParsed)
	}

	webcam, err := rimage.NewWebcamSource(nil)
	if err != nil {
		golog.Global.Fatal(err)
	}

	remoteView, err := gostream.NewView(x264.DefaultViewConfig)
	if err != nil {
		golog.Global.Fatal(err)
	}

	server := gostream.NewViewServer(port, remoteView, golog.Global)
	if err := server.Start(); err != nil {
		golog.Global.Fatal(err)
	}

	cancelCtx, cancelFunc := context.WithCancel(context.Background())
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		cancelFunc()
	}()

	gostream.StreamSource(cancelCtx, webcam, remoteView)

	if err := server.Stop(context.Background()); err != nil {
		golog.Global.Error(err)
	}
}
