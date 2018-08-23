package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"regexp"
	"strings"
	"time"

	upnp "github.com/NebulousLabs/go-upnp/goupnp"
	"github.com/gin-gonic/gin"
	ssdp "github.com/koron/go-ssdp"
	"github.com/spf13/viper"
	ccontext "github.com/tellytv/telly/context"
	"github.com/tellytv/telly/models"
)

func ServeLineup(cc *ccontext.CContext, exit chan bool, lineup *models.SQLLineup) {
	discoveryData := lineup.GetDiscoveryData()

	log.Debugln("creating device xml")
	upnp := discoveryData.UPNP()

	router := newGin()

	router.GET("/", deviceXML(upnp))
	router.GET("/device.xml", deviceXML(upnp))
	router.GET("/discover.json", discovery(discoveryData))
	router.GET("/lineup_status.json", lineupStatus(lineup)) // FIXME: replace bool with lineup.Scanning
	router.POST("/lineup.post", scanChannels(lineup))
	router.GET("/lineup.json", serveHDHRLineup(cc, lineup))
	router.GET("/lineup.xml", serveHDHRLineup(cc, lineup))
	router.GET("/auto/:channelID", stream(cc, lineup))

	baseAddr := fmt.Sprintf("%s:%d", lineup.ListenAddress, lineup.Port)

	if viper.GetBool("discovery.ssdp") {
		if _, ssdpErr := setupSSDP(baseAddr, lineup.Name, lineup.DeviceUUID); ssdpErr != nil {
			log.WithError(ssdpErr).Errorln("telly cannot advertise over ssdp")
		}
	}

	log.Infof(`telly lineup "%s" is live at http://%s/`, lineup.Name, baseAddr)

	srv := &http.Server{
		Addr:    baseAddr,
		Handler: router,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.WithError(err).Panicln("Error starting up web server")
		}
	}()

	for {
		select {
		case <-exit:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := srv.Shutdown(ctx); err != nil {
				log.WithError(err).Fatalln("error during tuner shutdown")
			}
			log.Warnln("Tuner restart commanded")
			return
		}
	}
}

func setupSSDP(baseAddress, deviceName, deviceUUID string) (*ssdp.Advertiser, error) {
	log.Debugf("Advertising telly as %s (%s)", deviceName, deviceUUID)

	adv, err := ssdp.Advertise(
		"upnp:rootdevice",
		fmt.Sprintf("uuid:%s::upnp:rootdevice", deviceUUID),
		fmt.Sprintf("http://%s/device.xml", baseAddress),
		deviceName,
		1800)

	if err != nil {
		return nil, err
	}

	go func(advertiser *ssdp.Advertiser) {
		aliveTick := time.Tick(15 * time.Second)

		for {
			select {
			case <-aliveTick:
				if err := advertiser.Alive(); err != nil {
					log.WithError(err).Panicln("error when sending ssdp heartbeat")
				}
			}
		}
	}(adv)

	return adv, nil
}

type dXMLContainer struct {
	upnp.RootDevice
	XMLName xml.Name `xml:"urn:schemas-upnp-org:device-1-0 root"`
}

func deviceXML(deviceXML upnp.RootDevice) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.XML(http.StatusOK, dXMLContainer{deviceXML, xml.Name{}})
	}
}

func discovery(data models.DiscoveryData) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, data)
	}
}

type hdhrLineupContainer struct {
	XMLName  xml.Name `xml:"Lineup"    json:"-"`
	Programs []models.HDHomeRunLineupItem
}

func serveHDHRLineup(cc *ccontext.CContext, lineup *models.SQLLineup) gin.HandlerFunc {
	return func(c *gin.Context) {

		channels, channelsErr := cc.API.LineupChannel.GetChannelsForLineup(lineup.ID, true)
		if channelsErr != nil {
			c.AbortWithError(http.StatusInternalServerError, channelsErr)
			return
		}

		hdhrItems := make([]models.HDHomeRunLineupItem, 0)
		for _, channel := range channels {
			hdhrItems = append(hdhrItems, *channel.HDHR)
		}

		if strings.HasSuffix(c.Request.URL.String(), ".xml") {
			buf, marshallErr := xml.MarshalIndent(hdhrLineupContainer{Programs: hdhrItems}, "", "\t")
			if marshallErr != nil {
				c.AbortWithError(http.StatusInternalServerError, fmt.Errorf("error marshalling lineup to XML"))
			}
			c.Data(http.StatusOK, "application/xml", []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+"\n"+string(buf)))
			return
		}
		c.JSON(http.StatusOK, hdhrItems)
	}
}

func stream(cc *ccontext.CContext, lineup *models.SQLLineup) gin.HandlerFunc {
	return func(c *gin.Context) {
		channelID := c.Param("channelID")[1:]

		channel, channelErr := cc.API.LineupChannel.GetLineupChannelByID(channelID)
		if channelErr != nil {
			c.AbortWithError(http.StatusInternalServerError, channelErr)
			return
		}

		log.Infof("Serving channel number %s", channelID)

		if !viper.IsSet("iptv.ffmpeg") {
			c.Redirect(http.StatusMovedPermanently, channel.VideoTrack.StreamURL)
			return
		}

		log.Infoln("Transcoding stream with ffmpeg")

		run := exec.Command("ffmpeg", "-re", "-i", channel.VideoTrack.StreamURL, "-codec", "copy", "-bsf:v", "h264_mp4toannexb", "-f", "mpegts", "-tune", "zerolatency", "-progress", "pipe:2", "pipe:1")
		ffmpegout, err := run.StdoutPipe()
		if err != nil {
			log.WithError(err).Errorln("StdoutPipe Error")
			return
		}

		stderr, stderrErr := run.StderrPipe()
		if stderrErr != nil {
			log.WithError(stderrErr).Errorln("Error creating ffmpeg stderr pipe")
		}

		if startErr := run.Start(); startErr != nil {
			log.WithError(startErr).Errorln("Error starting ffmpeg")
			return
		}

		go func() {
			scanner := bufio.NewScanner(stderr)
			scanner.Split(split)
			buf := make([]byte, 2)
			scanner.Buffer(buf, bufio.MaxScanTokenSize)

			for scanner.Scan() {
				line := scanner.Text()
				status := processFFMPEGStatus(line)
				if status != nil {
					fmt.Printf("\rFFMPEG Status: channel number: %d bitrate: %s frames: %s total time: %s speed: %s", channel.ID, status.CurrentBitrate, status.FramesProcessed, status.CurrentTime, status.Speed)
				}
			}
		}()

		continueStream := true

		c.Stream(func(w io.Writer) bool {
			defer func() {
				log.Infoln("Stopped streaming", channelID)
				if killErr := run.Process.Kill(); killErr != nil {
					panic(killErr)
				}
				continueStream = false
				return
			}()
			if _, copyErr := io.Copy(w, ffmpegout); copyErr != nil {
				log.WithError(copyErr).Errorln("Error when copying data")
				continueStream = false
				return false
			}
			return continueStream
		})

		return

		c.AbortWithError(http.StatusNotFound, fmt.Errorf("unknown channel number %d", channelID))
	}
}

func scanChannels(lineup *models.SQLLineup) gin.HandlerFunc {
	return func(c *gin.Context) {
		scanAction := c.Query("scan")
		if scanAction == "start" {
			// FIXME: Actually implement a scan...
			// if refreshErr := lineup.Scan(); refreshErr != nil {
			// 	c.AbortWithError(http.StatusInternalServerError, refreshErr)
			// }
			c.AbortWithStatus(http.StatusOK)
			return
		} else if scanAction == "abort" {
			c.AbortWithStatus(http.StatusOK)
			return
		}
		c.String(http.StatusBadRequest, "%s is not a valid scan command", scanAction)
	}
}

func lineupStatus(lineup *models.SQLLineup) gin.HandlerFunc {
	return func(c *gin.Context) {
		payload := LineupStatus{
			ScanInProgress: models.ConvertibleBoolean(false),
			ScanPossible:   models.ConvertibleBoolean(true),
			Source:         "Cable",
			SourceList:     []string{"Cable"},
		}
		// FIXME: Implement a scan param on SQLLineup.
		if false {
			payload = LineupStatus{
				ScanInProgress: models.ConvertibleBoolean(true),
				// Gotta fake out Plex.
				Progress: 50,
				Found:    50,
			}
		}

		c.JSON(http.StatusOK, payload)
	}
}

func split(data []byte, atEOF bool) (advance int, token []byte, spliterror error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		// We have a full newline-terminated line.
		return i + 1, data[0:i], nil
	}
	if i := bytes.IndexByte(data, '\r'); i >= 0 {
		// We have a cr terminated line
		return i + 1, data[0:i], nil
	}
	if atEOF {
		return len(data), data, nil
	}

	return 0, nil, nil
}

type FFMPEGStatus struct {
	FramesProcessed string
	CurrentTime     string
	CurrentBitrate  string
	Progress        float64
	Speed           string
}

func processFFMPEGStatus(line string) *FFMPEGStatus {
	status := new(FFMPEGStatus)
	if strings.Contains(line, "frame=") && strings.Contains(line, "time=") && strings.Contains(line, "bitrate=") {
		var re = regexp.MustCompile(`=\s+`)
		st := re.ReplaceAllString(line, `=`)

		f := strings.Fields(st)
		var framesProcessed string
		var currentTime string
		var currentBitrate string
		var currentSpeed string

		for j := 0; j < len(f); j++ {
			field := f[j]
			fieldSplit := strings.Split(field, "=")

			if len(fieldSplit) > 1 {
				fieldname := strings.Split(field, "=")[0]
				fieldvalue := strings.Split(field, "=")[1]

				if fieldname == "frame" {
					framesProcessed = fieldvalue
				}

				if fieldname == "time" {
					currentTime = fieldvalue
				}

				if fieldname == "bitrate" {
					currentBitrate = fieldvalue
				}
				if fieldname == "speed" {
					currentSpeed = fieldvalue
					if currentSpeed == "1x" {
						currentSpeed = "1.000x"
					}
				}
			}
		}

		status.CurrentBitrate = currentBitrate
		status.FramesProcessed = framesProcessed
		status.CurrentTime = currentTime
		status.Speed = currentSpeed
		return status
	}
	return nil
}