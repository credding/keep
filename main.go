package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	flag "github.com/spf13/pflag"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
)

var (
	flags *flag.FlagSet

	ttl  time.Duration
	help bool
)

func init() {
	flags = flag.NewFlagSet("keep", flag.ContinueOnError)
	flags.SortFlags = false
	flags.Usage = func() {
		_, _ = fmt.Fprint(os.Stderr,
			"Usage: keep [OPTION]... [--] COMMAND [ARG]...\n"+
				"Save the output of a command for recall later.\n")
		flags.PrintDefaults()
	}
	flags.SetInterspersed(false)
	flags.SetOutput(os.Stderr)

	flags.DurationVar(&ttl, "ttl", 12*time.Hour, "time to remember command output")
	flags.BoolVarP(&help, "help", "h", false, "display this help message")
}

func main() {
	err := run()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "keep: %s\n", err)
		os.Exit(1)
	}
}

func run() error {
	err := flags.Parse(os.Args[1:])
	if err != nil {
		return err
	}

	if help {
		flags.Usage()
		return nil
	}

	if len(flags.Args()) == 0 {
		return errors.New("no command to run")
	}

	return keep(flags.Args(), ttl)
}

func keep(command []string, ttl time.Duration) (err error) {
	state := &commandState{Cmd: command}

	stateFile, err := openStateFile(state.Key())
	if err != nil {
		return err
	}
	defer doCloseStateFile(stateFile, &err)

	err = readState(stateFile, state)
	if err != nil {
		return err
	}

	if !state.IsExpired(ttl) {
		_, err = io.Copy(os.Stdout, state.Output())
		if err != nil {
			return err
		}
		return nil
	}

	out, err := captureOutput(command[0], command[1:])
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		os.Exit(exitErr.ExitCode())
	}
	if err != nil {
		return err
	}

	state.SetOutput(out)

	err = writeState(stateFile, state)
	if err != nil {
		return err
	}

	return nil
}

func openStateFile(key string) (*os.File, error) {
	stateHome, err := xdgStateHomeDir()
	if err != nil {
		return nil, err
	}

	name := filepath.Join(stateHome, "keep", key)
	file, err := os.OpenFile(name, os.O_RDWR|os.O_CREATE, 0600)
	if errors.Is(err, os.ErrNotExist) {
		err = os.Mkdir(filepath.Dir(name), 0700)
		if err != nil {
			return nil, err
		}
		file, err = os.OpenFile(name, os.O_RDWR|os.O_CREATE, 0600)
	}
	if err != nil {
		return nil, err
	}

	err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX)
	if err != nil {
		_ = file.Close()
		return nil, err
	}

	return file, nil
}

func doCloseStateFile(file *os.File, err *error) {
	unlockErr := syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	if unlockErr != nil && *err == nil {
		*err = unlockErr
	}
	closeErr := file.Close()
	if closeErr != nil && *err == nil {
		*err = closeErr
	}
}

func readState(file *os.File, state *commandState) (err error) {
	dec := json.NewDecoder(file)
	err = dec.Decode(&state)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	_, err = file.Seek(0, 0)
	if err != nil {
		return err
	}

	return nil
}

func writeState(file *os.File, state *commandState) (err error) {
	err = file.Truncate(0)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(file)
	enc.SetEscapeHTML(false)
	err = enc.Encode(state)
	if err != nil {
		return err
	}
	return nil
}

func captureOutput(name string, args []string) ([]byte, error) {
	var buf bytes.Buffer

	cmd := exec.Command(name, args...)
	cmd.Stdout = io.MultiWriter(&buf, os.Stdout)
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	err := cmd.Run()
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func xdgStateHomeDir() (string, error) {
	dir := os.Getenv("XDG_STATE_HOME")
	if dir != "" {
		return dir, nil
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".local", "state"), nil
}

type commandState struct {
	Cmd       []string  `json:"cmd"`
	Time      time.Time `json:"time"`
	OutFormat string    `json:"outfmt,omitempty"`
	Out       string    `json:"out,omitempty"`
}

func (s *commandState) Key() string {
	argsSum := sha1.Sum([]byte(strings.Join(s.Cmd[1:], "\t")))
	return s.Cmd[0] + "_" + base64.RawURLEncoding.EncodeToString(argsSum[:])
}

func (s *commandState) IsExpired(ttl time.Duration) bool {
	return time.Now().After(s.Time.Add(ttl))
}

func (s *commandState) SetOutput(buf []byte) {
	s.Time = time.Now()
	if utf8.Valid(buf) {
		s.Out = string(buf)
	} else {
		s.OutFormat = "base64"
		s.Out = base64.StdEncoding.EncodeToString(buf)
	}
}

func (s *commandState) Output() io.Reader {
	var out io.Reader = strings.NewReader(s.Out)
	switch s.OutFormat {
	case "base64":
		return base64.NewDecoder(base64.StdEncoding, out)
	default:
		return out
	}
}
