package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

// Profile downloads a bounded private operator profile. Caller owns dst.
func (c *Client) Profile(ctx context.Context, kind string, seconds int, dst io.Writer) error {
	if kind != "heap" && kind != "goroutine" && kind != "cpu" {
		return errors.New("profile kind must be heap, goroutine or cpu")
	}
	if seconds < 1 || seconds > 5 {
		return errors.New("profile seconds must be 1..5")
	}
	path := "/v1/admin/profile?kind=" + kind
	if kind == "cpu" {
		path += "&seconds=" + strconv.Itoa(seconds)
	}
	req, err := http.NewRequestWithContext(ctx, "GET", c.base+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("User-Agent", c.agent)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("profile HTTP status %d", resp.StatusCode)
	}
	n, err := io.Copy(dst, io.LimitReader(resp.Body, (16<<20)+1))
	if err != nil {
		return err
	}
	if n > 16<<20 {
		return errors.New("profile exceeds 16 MiB")
	}
	if n == 0 {
		return errors.New("empty profile")
	}
	return nil
}
