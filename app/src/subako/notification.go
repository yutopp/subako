package subako

import (
	"log"
	"net/http"
	"bytes"
	"errors"
	"io/ioutil"
	"encoding/json"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
)


type NotificationConfig struct {
	TargetUrl		string
	Secret			string
}


type NotificationContext struct {
	TargetUrl		string
	Secret			string
}

func MakeNotificationContext(config *NotificationConfig) (*NotificationContext, error) {
	return &NotificationContext{
		TargetUrl: config.TargetUrl,
		Secret: config.Secret,
	}, nil
}

func (ctx *NotificationContext) PostUpdate(message interface{}) error {
	log.Printf("Send a notification to %s", ctx.TargetUrl)

	// generate payload
	payload, err := json.Marshal(message)
	if err != nil { return err }

	// generate signature
	mac := hmac.New(sha1.New, []byte(ctx.Secret))
	mac.Write([]byte(payload))
	generatedMAC := hex.EncodeToString(mac.Sum(nil))
	log.Printf("generated MAC => %s\n", generatedMAC)

	// make request
	req, err := http.NewRequest(
        "POST",
        ctx.TargetUrl,
        bytes.NewBuffer(payload),
	)
	if err != nil { return err }
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Torigoya-Factory-Signature", generatedMAC)

	// send!
	client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil { return err }
    defer resp.Body.Close()

	log.Println("sent")

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil { return err }

	if resp.StatusCode != 200 {
		return errors.New(string(body))
	} else {
		return nil
	}
}
