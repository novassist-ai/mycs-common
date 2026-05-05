package utils

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"os"

	"golang.org/x/crypto/ssh"
)

type remoteScriptType byte
type remoteShellType byte

const (
	cmdLine remoteScriptType = iota
	rawScript
	scriptFile

	interactiveShell remoteShellType = iota
	nonInteractiveShell
)

type SSHClient struct {
	client *ssh.Client
}

type SSHTerminalConfig struct {
	Term   string
	Height int
	Weight int
	Modes  ssh.TerminalModes
}

// starts a client connection to the given SSH server with passwd authmethod.
func SSHDialWithPasswd(
	address, user, passwd string,
) (*SSHClient, error) {

	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.Password(passwd),
		},
		HostKeyCallback: ssh.HostKeyCallback(
			func(hostname string, remote net.Addr, key ssh.PublicKey) error {
				return nil
			},
		),
	}
	return SSHDial("tcp", address, config)
}

// starts a client connection to the given SSH server with key authmethod.
func SSHDialWithKey(
	address, user string,
	key []byte,
) (*SSHClient, error) {

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, err
	}

	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.HostKeyCallback(
			func(hostname string, remote net.Addr, key ssh.PublicKey) error {
				return nil
			},
		),
	}
	return SSHDial("tcp", address, config)
}

// same as DialWithKey but with a passphrase to decrypt the private key
func SSHDialWithKeyWithPassphrase(
	address, user string,
	key []byte, passphrase string,
) (*SSHClient, error) {

	signer, err := ssh.ParsePrivateKeyWithPassphrase(key, []byte(passphrase))
	if err != nil {
		return nil, err
	}

	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.HostKeyCallback(
			func(hostname string, remote net.Addr, key ssh.PublicKey) error {
				return nil
			},
		),
	}
	return SSHDial("tcp", address, config)
}

// dial starts a client connection to the given SSH server.
func SSHDial(
	network, address string,
	config *ssh.ClientConfig,
) (*SSHClient, error) {

	client, err := ssh.Dial(network, address, config)
	if err != nil {
		return nil, err
	}
	return &SSHClient{
		client: client,
	}, nil
}

func (c *SSHClient) Close() error {
	return c.client.Close()
}

// run a command on client
func (c *SSHClient) Cmd(cmd string) *remoteScript {
	return &remoteScript{
		_type:  cmdLine,
		client: c.client,
		script: bytes.NewBufferString(cmd + "\n"),
	}
}

// run a script on a client
func (c *SSHClient) Script(script string) *remoteScript {
	return &remoteScript{
		_type:  rawScript,
		client: c.client,
		script: bytes.NewBufferString(script + "\n"),
	}
}

// run the given script file on a client
func (c *SSHClient) ScriptFile(fname string) *remoteScript {
	return &remoteScript{
		_type:      scriptFile,
		client:     c.client,
		scriptFile: fname,
	}
}

type remoteScript struct {
	client     *ssh.Client
	_type      remoteScriptType
	script     *bytes.Buffer
	scriptFile string
	err        error

	stdout io.Writer
	stderr io.Writer
}

// run commands over ssh client
func (rs *remoteScript) Run() error {

	if rs.err != nil {
		fmt.Println(rs.err)
		return rs.err
	}

	if rs._type == cmdLine {
		return rs.runCmds()
	} else if rs._type == rawScript {
		return rs.runScript()
	} else if rs._type == scriptFile {
		return rs.runScriptFile()
	} else {
		return errors.New("Not supported remoteScript type")
	}
}

// return output of ssh command or script run
func (rs *remoteScript) Output() ([]byte, error) {

	if rs.stdout != nil {
		return nil, errors.New("Stdout already set")
	}
	var out bytes.Buffer
	rs.stdout = &out
	err := rs.Run()
	return out.Bytes(), err
}

func (rs *remoteScript) SmartOutput() ([]byte, error) {

	if rs.stdout != nil {
		return nil, errors.New("Stdout already set")
	}
	if rs.stderr != nil {
		return nil, errors.New("Stderr already set")
	}

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)
	rs.stdout = &stdout
	rs.stderr = &stderr
	err := rs.Run()
	if err != nil {
		return stderr.Bytes(), err
	}
	return stdout.Bytes(), err
}

func (rs *remoteScript) Cmd(cmd string) *remoteScript {

	_, err := rs.script.WriteString(cmd + "\n")
	if err != nil {
		rs.err = err
	}
	return rs
}

func (rs *remoteScript) SetStdio(stdout, stderr io.Writer) *remoteScript {

	rs.stdout = stdout
	rs.stderr = stderr
	return rs
}

func (rs *remoteScript) runCmd(cmd string) error {

	session, err := rs.client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	session.Stdout = rs.stdout
	session.Stderr = rs.stderr

	if err := session.Run(cmd); err != nil {
		return err
	}
	return nil
}

func (rs *remoteScript) runCmds() error {

	for {
		statment, err := rs.script.ReadString('\n')
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if err := rs.runCmd(statment); err != nil {
			return err
		}
	}

	return nil
}

func (rs *remoteScript) runScript() error {

	session, err := rs.client.NewSession()
	if err != nil {
		return err
	}

	session.Stdin = rs.script
	session.Stdout = rs.stdout
	session.Stderr = rs.stderr

	if err := session.Shell(); err != nil {
		return err
	}
	if err := session.Wait(); err != nil {
		return err
	}

	return nil
}

func (rs *remoteScript) runScriptFile() error {

	var buffer bytes.Buffer
	file, err := os.Open(rs.scriptFile)
	if err != nil {
		return err
	}
	_, err = io.Copy(&buffer, file)
	if err != nil {
		return err
	}

	rs.script = &buffer
	return rs.runScript()
}

// remote shell

type remoteShell struct {
	client *ssh.Client

	stdin          io.Reader
	stdout, stderr io.Writer

	terminalConfig *SSHTerminalConfig
	requestPty     bool
}

// create an interactive shell on client
func (c *SSHClient) Terminal(config *SSHTerminalConfig) *remoteShell {

	return &remoteShell{
		client: c.client,

		stdin:  os.Stdin,
		stdout: os.Stdout,
		stderr: os.Stderr,

		terminalConfig: config,
		requestPty:     true,
	}
}

// create a noninteractive shell on client
func (c *SSHClient) Shell() *remoteShell {

	return &remoteShell{
		client: c.client,

		stdin:  os.Stdin,
		stdout: os.Stdout,
		stderr: os.Stderr,

		terminalConfig: nil,
		requestPty:     false,
	}
}

// override default system standard input and output
func (rs *remoteShell) SetStdio(
	stdin io.Reader,
	stdout, stderr io.Writer,
) *remoteShell {

	rs.stdin = stdin
	rs.stdout = stdout
	rs.stderr = stderr
	return rs
}

// Start start a remote shell on client
func (rs *remoteShell) Start() error {

	var (
		err error

		session *ssh.Session
	)

	if session, err = rs.client.NewSession(); err != nil {
		return err
	}
	defer session.Close()

	session.Stdin = rs.stdin
	session.Stdout = rs.stdout
	session.Stderr = rs.stderr

	if rs.requestPty {
		tc := rs.terminalConfig
		if tc == nil {
			tc = &SSHTerminalConfig{
				Term:   "xterm",
				Height: 40,
				Weight: 80,
			}
		}
		if err = session.RequestPty(tc.Term, tc.Height, tc.Weight, tc.Modes); err != nil {
			return err
		}
	}

	if err := session.Shell(); err != nil {
		return err
	}
	if err := session.Wait(); err != nil {
		return err
	}
	return nil
}
