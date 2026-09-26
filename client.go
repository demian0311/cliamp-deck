package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/bjarneo/cliamp/ipc"
)

// client talks to a running cliamp (TUI or --daemon) over its V2 socket.
// Every call opens a fresh connection; on a Unix socket that costs less than
// keeping request/response pairing straight across goroutines.
type client struct{ sock string }

func (c client) send(req ipc.V2Request) (ipc.V2Response, error) {
	req.ID = json.RawMessage(`"deck"`)
	resp, err := ipc.SendV2WithDeadline(c.sock, req, 3*time.Second)
	if err != nil {
		return resp, err
	}
	if !resp.OK {
		if resp.Error != nil {
			return resp, resp.Error
		}
		return resp, fmt.Errorf("%s%s failed", req.Method, req.Operation)
	}
	return resp, nil
}

func (c client) state() (*ipc.RuntimeSnapshot, error) {
	resp, err := c.send(ipc.V2Request{Method: "state.get"})
	if err != nil {
		return nil, err
	}
	return resp.Snapshot, nil
}

func (c client) spectrum() ([]float64, error) {
	resp, err := c.send(ipc.V2Request{Method: "spectrum.get"})
	if err != nil {
		return nil, err
	}
	var r ipc.Response
	if err := json.Unmarshal(resp.Result, &r); err != nil {
		return nil, err
	}
	return r.Bands, nil
}

// op submits an operation and waits for its job to reach a terminal state.
// The returned bytes are the job's result: an ipc.Response in JSON.
func (c client) op(name string, params any) (json.RawMessage, error) {
	var raw json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		raw = b
	}
	resp, err := c.send(ipc.V2Request{Method: "operation.submit", Operation: name, Params: raw})
	if err != nil {
		return nil, err
	}
	job := resp.Job
	deadline := time.Now().Add(30 * time.Second)
	for job != nil && !jobDone(job.State) {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("%s timed out", name)
		}
		time.Sleep(40 * time.Millisecond)
		r, err := c.send(ipc.V2Request{Method: "job.get", JobID: job.ID})
		if err != nil {
			return nil, err
		}
		job = r.Job
	}
	if job == nil {
		return resp.Result, nil
	}
	if job.State != ipc.JobSucceeded {
		if job.Error != nil {
			return nil, job.Error
		}
		return nil, fmt.Errorf("%s %s", name, job.State)
	}
	return job.Result, nil
}

func jobDone(s ipc.JobState) bool {
	return s == ipc.JobSucceeded || s == ipc.JobFailed || s == ipc.JobCanceled
}

func (c client) providers() ([]ipc.ProviderInfo, error) {
	raw, err := c.op("provider.list", nil)
	if err != nil {
		return nil, err
	}
	var r ipc.Response
	err = json.Unmarshal(raw, &r)
	return r.Providers, err
}

func (c client) playlists(provider string) ([]ipc.PlaylistInfo, error) {
	raw, err := c.op("provider.playlists", map[string]any{"provider": provider, "limit": 200})
	if err != nil {
		return nil, err
	}
	var r ipc.Response
	err = json.Unmarshal(raw, &r)
	return r.Playlists, err
}

// catalog loads the next page of a paged catalog (Radio's station directory)
// and returns every entry loaded so far, plus how many the page added.
func (c client) catalog(provider string, offset, limit int) ([]ipc.PlaylistInfo, int, error) {
	raw, err := c.op("provider.catalog", map[string]any{"provider": provider, "offset": offset, "limit": limit})
	if err != nil {
		return nil, 0, err
	}
	var r ipc.Response
	err = json.Unmarshal(raw, &r)
	return r.Playlists, r.Total, err
}

func (c client) alive() bool {
	_, err := c.state()
	return err == nil
}

// spawnDaemon starts `cliamp --daemon` in its own session so it outlives us,
// then waits for the socket to answer.
func spawnDaemon(c client) error {
	cmd := exec.Command("cliamp", "--daemon")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	devnull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer devnull.Close()
	cmd.Stdin, cmd.Stdout, cmd.Stderr = devnull, devnull, devnull
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start cliamp --daemon: %w", err)
	}
	_ = cmd.Process.Release()
	for range 50 {
		if c.alive() {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("cliamp --daemon started but %s never answered", c.sock)
}
