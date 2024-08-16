package server

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/maurice2k/tcpserver"
)

type Server struct {
	*tcpserver.Server
}

var default_host *net.IP

type options struct {
	host                   *net.IP
	port                   *int
	socketReusePort        *bool
	socketFastOpen         *bool
	socketFastOpenQueueLen *int
	socketDeferAccept      *bool
	loops                  *int
	workerpoolShards       *int
	allowThreadLocking     *bool
	ballast                *int
	handler                tcpserver.RequestHandlerFunc
}

type Option func(option *options) error

func init() {
	default_host = new(net.IP)
	if err := default_host.UnmarshalText([]byte("127.0.0.1")); err != nil {
		panic(err)
	}
}

func New(opts ...Option) (*Server, error) {
	var opt options
	for _, option := range opts {
		if err := option(&opt); err != nil {
			return nil, err
		}
	}
	host := default_host
	if opt.host != nil {
		host = opt.host
	}
	port := 0
	if opt.port != nil {
		port = *opt.port
	}

	address := fmt.Sprintf("%s:%d", *host, port)
	if _, err := net.ResolveTCPAddr("tcp", address); err != nil {
		return nil, fmt.Errorf("validate tcp server address: %w", err)
	}

	srv, err := tcpserver.NewServer(address)
	if err != nil {
		return nil, fmt.Errorf("creates a new server instance: %w", err)
	}

	cfg := new(tcpserver.ListenConfig)
	if opt.socketReusePort != nil {
		cfg.SocketReusePort = *opt.socketReusePort
	}
	if opt.socketFastOpen != nil {
		cfg.SocketFastOpen = *opt.socketFastOpen
	}
	if opt.socketFastOpenQueueLen != nil {
		cfg.SocketFastOpenQueueLen = *opt.socketFastOpenQueueLen
	}
	if opt.loops != nil {
		srv.SetLoops(*opt.loops)
	}
	if opt.workerpoolShards != nil {
		srv.SetWorkerpoolShards(*opt.workerpoolShards)
	}
	if opt.allowThreadLocking != nil {
		srv.SetAllowThreadLocking(*opt.allowThreadLocking)
	}
	if opt.ballast != nil {
		srv.SetBallast(*opt.ballast)
	}
	// srv.SetMaxAcceptConnections()
	// srv.SetContext(&ctx)
	// srv.SetTLSConfig()

	if opt.handler != nil {
		srv.SetRequestHandler(opt.handler)
	}

	srv.SetListenConfig(cfg)

	return &Server{srv}, nil
}

func (s *Server) Start() error {
	if err := s.Listen(); err != nil {
		return fmt.Errorf("error listening on interface: %w", err)
	}

	go func() {
		s.Serve()
	}()

	return nil
}

func (s *Server) AwaitStopSignal(stopTimeout time.Duration) (os.Signal, error) {
	c := make(chan os.Signal, 1)
	signal.Notify(c,
		os.Interrupt,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGHUP,
		syscall.SIGQUIT,
		syscall.SIGABRT)
	sig := <-c

	return sig, s.Shutdown(stopTimeout)
}

// default host=127.0.0.1
func WithHost(host string) Option {
	return func(options *options) error {
		ip := new(net.IP)
		if host == "" || host == "localhost" {
			ip = default_host
		} else {
			if err := ip.UnmarshalText([]byte(host)); err != nil {
				return err
			}
		}
		options.host = ip
		return nil
	}
}

// if port=0 listening to random available port
func WithPort(port int) Option {
	return func(options *options) error {
		if port < 0 {
			return fmt.Errorf("port cannot be less than zero")
		}
		options.port = &port
		return nil
	}
}

// Enable/disable SO_REUSEPORT  (requires Linux >=2.4)
func WithSocketReusePort(enable bool) Option {
	return func(options *options) error {
		options.socketReusePort = &enable
		return nil
	}
}

// Enable/disable TCP_FASTOPEN (requires Linux >=3.7 or Windows 10, version 1607)
// For Linux:
// - see https://lwn.net/Articles/508865/
// - enable with "echo 3 >/proc/sys/net/ipv4/tcp_fastopen" for client and server
// For Windows:
// - enable with "netsh int tcp set global fastopen=enabled"
func WithSocketFastOpen(enable bool) Option {
	return func(options *options) error {
		options.socketFastOpen = &enable
		return nil
	}
}

// Queue length for TCP_FASTOPEN (default 256)
func WithSocketFastOpenQueueLen(len int) Option {
	return func(options *options) error {
		if len < 0 {
			return fmt.Errorf("queue length cannot be less than zero")
		}
		options.socketFastOpenQueueLen = &len
		return nil
	}
}

// Enable/disable TCP_DEFER_ACCEPT (requires Linux >=2.4)
func WithSocketDeferAccept(enable bool) Option {
	return func(options *options) error {
		options.socketDeferAccept = &enable
		return nil
	}
}

// Sets number of accept loops. Defaults to 4 which is more than enough for most use cases
func WithLoops(loops int) Option {
	return func(options *options) error {
		options.loops = &loops
		return nil
	}
}

// Sets number of workerpool shards. Defaults to GOMAXPROCS*2
func WithWorkerpoolShards(shards int) Option {
	return func(options *options) error {
		options.workerpoolShards = &shards
		return nil
	}
}

// Whether or not allow thread locking in accept loops
func WithAllowThreadLocking(enable bool) Option {
	return func(options *options) error {
		options.allowThreadLocking = &enable
		return nil
	}
}

// This is kind of a hack to reduce GC cycles. Normally Go's GC kicks in whenever used memory doubles (see runtime.SetGCPercent() or GOGC env var: https://golang.org/pkg/runtime/debug/#SetGCPercent). With a very low memory footprint this might dramatically impact your performance, especially with lots of connections coming in waves.
func WithBallast(sizeInMiB int) Option {
	return func(options *options) error {
		if sizeInMiB <= 0 {
			return fmt.Errorf("ballast cannot be less than or equal to zero")
		}
		options.ballast = &sizeInMiB
		return nil
	}
}

func WithRequestHandler(f tcpserver.RequestHandlerFunc) Option {
	return func(options *options) error {
		options.handler = f
		return nil
	}
}
