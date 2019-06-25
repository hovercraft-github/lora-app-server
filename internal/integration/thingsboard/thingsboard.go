package thingsboard

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/lora-app-server/internal/integration"
)

// Config holds the Thingsboard integration configuration.
type Config struct {
	Server string `json:"server"`
}

// Validate validates the Config.
func (c Config) Validate() error {
	return nil
}

// Integration implements the Thingsboard integration.
type Integration struct {
	server string
}

// New creates a new Thingsboard integration.
func New(conf Config) (*Integration, error) {
	return &Integration{
		server: conf.Server,
	}, nil
}

// SendDataUp sends the (decoded) uplink payload to the Thingsboard endpoint.
func (i *Integration) SendDataUp(pl integration.DataUpPayload) error {
	accessToken, ok := pl.Variables["ThingsBoardAccessToken"]
	if !ok {
		log.WithField("dev_eui", pl.DevEUI).Warning("integration/thingsboard: device does not have a 'ThingsBoardAccessToken' variable")
		return nil
	}

	data := structToMap("data", pl.Object)
	for k, v := range pl.Tags {
		data[k] = v
	}
	data["application_name"] = pl.ApplicationName
	data["application_id"] = strconv.FormatInt(pl.ApplicationID, 10)
	data["device_name"] = pl.DeviceName
	data["dev_eui"] = pl.DevEUI
	data["f_port"] = pl.FPort

	if err := i.send(accessToken, data); err != nil {
		return errors.Wrap(err, "send event error")
	}

	log.WithFields(log.Fields{
		"event":   "up",
		"dev_eui": pl.DevEUI,
	}).Info("integration/thingsboard: telemetry uploaded")

	return nil
}

// SendJoinNotification returns nil.
func (i *Integration) SendJoinNotification(pl integration.JoinNotification) error {
	return nil
}

// SendACKNotification returns nil.
func (i *Integration) SendACKNotification(pl integration.ACKNotification) error {
	return nil
}

// SendErrorNotification returns nil.
func (i *Integration) SendErrorNotification(pl integration.ErrorNotification) error {
	return nil
}

// SendStatusNotification sends the device-status fields to the Thingsboard endpoint.
func (i *Integration) SendStatusNotification(pl integration.StatusNotification) error {
	accessToken, ok := pl.Variables["ThingsBoardAccessToken"]
	if !ok {
		log.WithField("dev_eui", pl.DevEUI).Warning("integration/thingsboard: device does not have a 'ThingsBoardAccessToken' variable")
		return nil
	}

	data := map[string]interface{}{
		"application_name":                 pl.ApplicationName,
		"application_id":                   strconv.FormatInt(pl.ApplicationID, 10),
		"device_name":                      pl.DeviceName,
		"dev_eui":                          pl.DevEUI,
		"status_battery":                   pl.Battery,
		"status_margin":                    pl.Margin,
		"status_external_power_source":     pl.ExternalPowerSource,
		"status_battery_level":             pl.BatteryLevel,
		"status_battery_level_unavailable": pl.BatteryLevelUnavailable,
	}
	for k, v := range pl.Tags {
		data[k] = v
	}

	if err := i.send(accessToken, data); err != nil {
		return errors.Wrap(err, "send event error")
	}

	log.WithFields(log.Fields{
		"event":   "status",
		"dev_eui": pl.DevEUI,
	}).Info("integration/thingsboard: telemetry uploaded")

	return nil
}

// SendLocationNotification sends the device location to the Thingsboard endpoint.
func (i *Integration) SendLocationNotification(pl integration.LocationNotification) error {
	accessToken, ok := pl.Variables["ThingsBoardAccessToken"]
	if !ok {
		log.WithField("dev_eui", pl.DevEUI).Warning("integration/thingsboard: device does not have a 'ThingsBoardAccessToken' variable")
		return nil
	}

	data := map[string]interface{}{
		"application_name":   pl.ApplicationName,
		"application_id":     strconv.FormatInt(pl.ApplicationID, 10),
		"device_name":        pl.DeviceName,
		"dev_eui":            pl.DevEUI,
		"location_latitude":  pl.Location.Latitude,
		"location_longitude": pl.Location.Longitude,
		"location_altitude":  pl.Location.Altitude,
	}
	for k, v := range pl.Tags {
		data[k] = v
	}

	if err := i.send(accessToken, data); err != nil {
		return errors.Wrap(err, "send event error")
	}

	log.WithFields(log.Fields{
		"event":   "location",
		"dev_eui": pl.DevEUI,
	}).Info("integration/thingsboard: telemetry uploaded")

	return nil
}

// DataDownChan returns nil.
func (i *Integration) DataDownChan() chan integration.DataDownPayload {
	return nil
}

// Close returns nil.
func (i *Integration) Close() error {
	return nil
}

func (i *Integration) send(token string, data map[string]interface{}) error {
	b, err := json.Marshal(data)
	if err != nil {
		return errors.Wrap(err, "marshal json error")
	}

	url := fmt.Sprintf("%s/api/v1/%s/telemetry", i.server, token)
	req, err := http.NewRequest("POST", url, bytes.NewReader(b))
	if err != nil {
		return errors.Wrap(err, "new request error")
	}

	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "http request error")
	}
	defer resp.Body.Close()

	// check that response is in 200 range
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("expected 2xx response, got: %d (%s)", resp.StatusCode, string(b))
	}

	return nil
}

func structToMap(prefix string, v interface{}) map[string]interface{} {
	out := make(map[string]interface{})

	if v == nil {
		return out
	}

	switch o := v.(type) {
	case int, uint, float32, float64, uint8, int8, uint16, int16, uint32, int32, uint64, int64, string, bool:
		out[prefix] = o
	default:
		switch reflect.TypeOf(o).Kind() {
		case reflect.Map:
			v := reflect.ValueOf(o)
			keys := v.MapKeys()

			for _, k := range keys {
				keyName := fmt.Sprintf("%v", k.Interface())

				for k, v := range structToMap(prefix+"_"+keyName, v.MapIndex(k).Interface()) {
					out[k] = v
				}
			}

		case reflect.Struct:
			v := reflect.ValueOf(o)
			l := v.NumField()

			for i := 0; i < l; i++ {
				if !v.Field(i).CanInterface() {
					continue
				}

				fieldName := v.Type().Field(i).Tag.Get("influxdb")
				if fieldName == "" {
					fieldName = strings.ToLower(v.Type().Field(i).Name)
				}

				for k, v := range structToMap(prefix+"_"+fieldName, v.Field(i).Interface()) {
					out[k] = v
				}
			}

		case reflect.Ptr:
			v := reflect.Indirect(reflect.ValueOf(o))
			for k, v := range structToMap(prefix, v.Interface()) {
				out[k] = v
			}

		default:
			log.WithField("type_name", fmt.Sprintf("%T", o)).Warning("integration/thingsboard: unhandled type!")
		}
	}

	return out
}
