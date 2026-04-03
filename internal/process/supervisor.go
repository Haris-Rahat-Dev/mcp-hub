package process

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/harisbinrahat/mcp-hub/internal/config"
)

type Supervisor struct {
	Logger *log.Logger
}

type ManagedProcess struct {
	name      string
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	done      chan error
	sendMu    sync.Mutex
	closeOnce sync.Once
	closed    atomic.Bool
}

func (s *Supervisor) Start(name string, server config.ServerConfig, onMessage func([]byte)) (*ManagedProcess, error) {
	cmd := exec.Command(server.Command, server.Args...)
	cmd.Env = append(os.Environ(), flattenEnv(server.Env)...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	proc := &ManagedProcess{
		name:  name,
		cmd:   cmd,
		stdin: stdin,
		done:  make(chan error, 1),
	}

	go readStdout(stdout, onMessage)
	go s.readStderr(name, stderr)
	go func() {
		err := cmd.Wait()
		proc.done <- err
		close(proc.done)
	}()

	return proc, nil
}

func (p *ManagedProcess) Send(raw []byte) error {
	if p.closed.Load() {
		return errors.New("process is closed")
	}
	p.sendMu.Lock()
	defer p.sendMu.Unlock()
	if _, err := p.stdin.Write(raw); err != nil {
		return err
	}
	_, err := p.stdin.Write([]byte("\n"))
	return err
}

func (p *ManagedProcess) Done() <-chan error {
	return p.done
}

func (p *ManagedProcess) PID() int {
	if p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}

func (p *ManagedProcess) Close() error {
	var closeErr error
	p.closeOnce.Do(func() {
		p.closed.Store(true)
		if p.cmd.Process == nil {
			return
		}
		if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
			closeErr = err
			_ = p.cmd.Process.Kill()
			return
		}
		go func() {
			select {
			case <-p.done:
			case <-time.After(2 * time.Second):
				_ = p.cmd.Process.Kill()
			}
		}()
	})
	return closeErr
}

func flattenEnv(values map[string]string) []string {
	out := make([]string, 0, len(values))
	for key, value := range values {
		out = append(out, fmt.Sprintf("%s=%s", key, value))
	}
	return out
}

func readStdout(stdout io.Reader, onMessage func([]byte)) {
	decoder := json.NewDecoder(stdout)
	for {
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return
		}
		if len(raw) == 0 {
			continue
		}
		if onMessage != nil {
			onMessage(append([]byte(nil), raw...))
		}
	}
}

func (s *Supervisor) readStderr(name string, stderr io.Reader) {
	if s.Logger == nil {
		return
	}
	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		s.Logger.Printf("[%s stderr] %s", name, scanner.Text())
	}
}
