# Telly

A IPTV proxy for Plex Live written in Golang

```
services:
  telly:
    image: ceramicwhite/telly:latest
    restart: unless-stopped
    container_name: telly
    volumes:
      - './data:/config'
      - '/etc/localtime:/etc/localtime:ro'
      - './data/logs:/var/log/telly'
    environment:
      - 'STREAMS=1'                                                  # Number of simultaneous streams that the telly virtual DVR will be able to provide
      - 'FFMPEG=true'                                                # If true, streams are buffered through ffmpeg; ffmpeg must be on your $PATH
      - 'LOG_REQUESTS=true'                                          # Log HTTP requests made to telly
      - 'BASE_ADDRESS=0.0.0.0:6077'                                  # IP address of the machine telly runs on
      - 'LISTEN_ADDRESS=0.0.0.0:6077'                                # This can stay as-is
      - 'PGID=1010'                                                  # Group ID for the process
      - 'PUID=1010'                                                  # User ID for the process
      ####### Only one of the following SOURCES should be set #########
                  #### Source 1 ####
      # - 'SOURCE1_NAME='                                            # Optional name for logging purposes
      # - 'SOURCE1_PROVIDER=Vaders'                                  # Provider name for the first source
      # - 'SOURCE1_USERNAME='                                        # Username for the first source
      # - 'SOURCE1_PASSWORD='                                        # Password for the first source
      # - 'SOURCE1_FILTER=Sports|Premium Movies|United States.*|USA' # Filter for the first source
      # - 'SOURCE1_FILTER_KEY=tvg-name'                              # Filter key for the first source
      # - 'SOURCE1_FILTER_RAW=false'                                 # If true, applies regex filter to entire line
      # - 'SOURCE1_SORT=group-title'                                 # Sort key for the first source
      #             #### Source 2 ####
      # - 'SOURCE2_NAME='                                            # Name for the second source
      # - 'SOURCE2_PROVIDER=IPTV-EPG'                                # Provider name for the second source
      # - 'SOURCE2_USERNAME=M3U-Identifier'                          # Username for the second source
      # - 'SOURCE2_PASSWORD=XML-Identifier'                          # Password for the second source
                  #### Source 3 ####
      - 'SOURCE3_PROVIDER=Custom'                                    # Provider name for the third source
      - 'SOURCE3_M3U=http://myprovider.com/playlist.m3u'             # M3U URL for the third source
      - 'SOURCE3_EPG=http://myprovider.com/epg.xml'                  # EPG URL for the third source
      #### Optional ####
      - 'DEVICE_AUTH=telly123'                                       # Device authentication for Plex
      - 'DEVICE_ID=12345678'                                         # Device ID for Plex
      - 'DEVICE_UUID='                                               # Device UUID (optional)
      - 'DEVICE_FIRMWARE_NAME=hdhomeruntc_atsc'                      # Device firmware name
      - 'DEVICE_FIRMWARE_VERSION=20150826'                           # Device firmware version
      - 'DEVICE_FRIENDLY_NAME=telly'                                 # Friendly name of the device
      - 'DEVICE_MANUFACTURER=Silicondust'                            # Device manufacturer
      - 'DEVICE_MODEL_NUMBER=HDTC-2US'                               # Device model number
      - 'SSDP=true'                                                  # Enable or disable SSDP
      - 'STARTING_CHANNEL=10000'                                     # Starting channel number for telly
      - 'XMLTV_CHANNELS=true'                                        # If true, uses channel numbers from M3U file
      - 'LOG_LEVEL=info'                                             # Log level [debug, info, warn, error, fatal]
      - 'SCHEDULES_DIRECT_USERNAME='                                 # Schedules Direct account username
      - 'SCHEDULES_DIRECT_PASSWORD='                                 # Schedules Direct account password
    ports:
      - '6077:6077'
```
