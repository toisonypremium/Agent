package notify

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"time"
)

func Ntfy(ctx context.Context, topic, text string) error {
	if topic == "" {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://ntfy.sh/"+topic, bytes.NewBufferString(text))
	if err != nil {
		return err
	}
	client := http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("ntfy http %d", resp.StatusCode)
	}
	return nil
}
