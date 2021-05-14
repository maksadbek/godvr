package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"godvr/internal/dvrip"
)

var (
	address       = flag.String("address", "192.168.1.147", "camera address: 192.168.1.147, 192.168.1.147:34567")
	name          = flag.String("name", "camera1", "name of the camera")
	outPath       = flag.String("out", "./", "output path that video files will be kept")
	chunkInterval = flag.Duration("chunkInterval", time.Minute*10, "time when application must create a new files")
	stream        = flag.String("stream", "Main", "camera stream name")
	user          = flag.String("user", "admin", "username")
	password      = flag.String("password", "", "password for the user")
	retryTime     = flag.Duration("retryTime", time.Second*5, "retry to connect if problem occur")
	debugMode     = flag.Bool("debug", false, "debug mode")
)

func main() {
	flag.Parse()

	settings := dvrip.Settings{
		Address:  *address,
		User:     *user,
		Password: *password,
		Debug:    *debugMode,
	}

	err := setupLogs()
	if err != nil {
		log.Print("warning: failed to setup a log file:", err)
	}

	settings.SetDefaults()
	log.Printf("using the following settings: %+v", settings)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, os.Kill)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		select {
		case <-stop:
			fmt.Println("received interrupt signal")
			cancel()
		}
	}()

	for {
		err := monitor(ctx, settings)
		if err == nil {
			break
		}

		debugf("fatal error: %v", err)
		log.Printf("camera is lost, wait %v and try again", *retryTime)

		select {
		case <-time.Tick(*retryTime):
		case <-ctx.Done():
			log.Print("done")
			return
		}
	}

}

func debugf(msg string, args ...interface{}) {
	if *debugMode {
		log.Printf(msg, args...)
	}
}

func monitor(ctx context.Context, settings dvrip.Settings) error {
	conn, err := dvrip.New(ctx, settings)
	if err != nil {
		debugf("failed to initiate connection: %v", err)
		return err
	}

	err = conn.Login()
	if err != nil {
		log.Print("failed to login: ", err)
		return err
	}

	log.Print("successfully logged in")

	err = conn.SetKeepAlive()
	if err != nil {
		log.Print("failed to set keepalive:", err)
		return err
	}

	log.Print("successfully set keepalive")

	err = conn.SetTime()
	if err != nil {
		log.Print("failed to set time:", err)
		return err
	}

	log.Print("successfully synced time")

	outChan := make(chan *dvrip.Frame)
	var videoFile, audioFile *os.File

	err = conn.Monitor(*stream, outChan)
	if err != nil {
		log.Print("failed to start monitoring:", err)
		return err
	}

	videoFile, audioFile, err = createChunkFiles(time.Now())
	if err != nil {
		return err
	}

	prevTime := time.Now()

	for {
		select {
		case frame, ok := <-outChan:
			if !ok {
				return conn.MonitorErr
			}

			now := time.Now()

			if prevTime.Add(*chunkInterval).Before(now) {
				errs := closeFiles(videoFile, audioFile)
				if err != nil {
					log.Printf("error occurred: %v", errs)
				}

				videoFile, audioFile, err = createChunkFiles(now)
				prevTime = now
			}

			processFrame(frame, audioFile, videoFile)
		case <-ctx.Done():
			errs := closeFiles(videoFile, audioFile)
			if err != nil {
				log.Printf("error occurred: %v", errs)
			}

			log.Print("done")
			return nil
		}
	}
}

func processFrame(frame *dvrip.Frame, audioFile, videoFile *os.File) {
	if frame.Meta.Type == "G711A" { // audio
		_, err := audioFile.Write(frame.Data)
		if err != nil {
			log.Println("warning: failed to write to file", err)
		}

		return
	}

	if frame.Meta.Frame != "" {
		_, err := videoFile.Write(frame.Data)
		if err != nil {
			log.Println("warning: failed to write to file", err)
		}

		return
	}

	return
}

func closeFiles(files ...*os.File) (errs []error) {
	for _, f := range files {
		err := f.Close()
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to close file: %v cause: %v", f.Name(), err))
		}
	}

	return
}

func createChunkFiles(t time.Time) (*os.File, *os.File, error) {
	dir := *outPath + "/" + (*name) + t.Format("/2006/01/02/")

	err := os.MkdirAll(dir, os.ModeDir)
	if err != nil {
		return nil, nil, err
	}

	file := *outPath + "/" + (*name) + t.Format("/2006/01/02/15.04.05")
	log.Print("starting files:", file)

	videoFile, err := os.Create(file + ".video")
	if err != nil {
		return nil, nil, err
	}

	audioFile, err := os.Create(file + ".audio")
	if err != nil {
		return nil, nil, err
	}

	return videoFile, audioFile, nil
}

func setupLogs() error {
	outDir := *outPath + "/" + *name
	err := os.Mkdir(outDir, os.ModeDir)

	if err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}

	logsFile, err := os.OpenFile(outDir+"/"+"logs.log", os.O_RDWR|os.O_APPEND|os.O_CREATE, 0666)
	if err != nil {
		return err
	}

	log.SetOutput(logsFile)

	return nil
}
