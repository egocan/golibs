package rotator

import (
	"github.com/egocan/golibs/strftime"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type (
	// A RotateWriter writes message to a set of output files.
	RotateWriter struct {
		pattern *strftime.Strftime // given pattern
		path    string             // current file path
		symlink *strftime.Strftime // symbolic link to current file path
		fp      *os.File           // current file pointer
		loc     *time.Location
		mux     sync.Locker
		log     logger
		init    bool // if true, open the file when New() method is called
	}

	// A Option with RotateWriter.
	Option func(*RotateWriter)
)

var (
	_   io.WriteCloser = (*RotateWriter)(nil) // check if object implements interface
	now                = time.Now             // for test
)

// New returns a RotateWriter with the given pattern and options.
func New(pattern string, options ...Option) (*RotateWriter, error) {
	p, err := strftime.New(pattern)
	if err != nil {
		return nil, err
	}

	c := &RotateWriter{
		pattern: p,
		path:    "",
		symlink: nil,
		fp:      nil,
		loc:     time.Local,
		mux:     new(sync.Mutex), // default mutex enable
		log:     &nopLogger{},
		init:    false,
	}

	for _, option := range options {
		option(c)
	}

	if c.init {
		if _, err := c.Write([]byte("")); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// MustNew is a convenience function equivalent to New that panics on failure
// instead of returning an error.
func MustNew(pattern string, options ...Option) *RotateWriter {
	c, err := New(pattern, options...)
	if err != nil {
		panic(err)
	}
	return c
}

// WithLocation set the location to loc.
func WithLocation(loc *time.Location) Option {
	return func(c *RotateWriter) {
		c.loc = loc
	}
}

// WithSymlink enables its creates a symbolic link to the specify pattern.
func WithSymlink(pattern string) Option {
	return func(c *RotateWriter) {
		p, err := strftime.New(pattern)
		if err != nil {
			panic(err)
		}
		c.symlink = p
	}
}

// WithMutex enables its uses sync.Mutex when file writing.
func WithMutex() Option {
	return func(c *RotateWriter) {
		c.mux = new(sync.Mutex)
	}
}

// WithNopMutex disables its uses sync.Mutex when file writing.
func WithNopMutex() Option {
	return func(c *RotateWriter) {
		c.mux = new(nopMutex)
	}
}

// WithDebug enables output stdout and stderr.
func WithDebug() Option {
	return func(c *RotateWriter) {
		c.log = newDebugLogger()
	}
}

// WithStdout enables output always stdout.
func WithStdout() Option {
	return func(c *RotateWriter) {
		c.log = newStdoutLogger()
	}
}

// WithStderr enables output always stderr.
func WithStderr() Option {
	return func(c *RotateWriter) {
		c.log = newStderrLogger()
	}
}

// WithInit enables its creates output file when RotateWriter initialize.
func WithInit() Option {
	return func(c *RotateWriter) {
		c.init = true
	}
}

// Write writes to the file and rotate files automatically based on current date and time.
func (c *RotateWriter) Write(b []byte) (int, error) {
	c.mux.Lock()
	defer c.mux.Unlock()

	t := now().In(c.loc)
	path := c.pattern.FormatString(t)

	if c.path != path {
		// close file
		go func(fp *os.File) {
			if fp == nil {
				return
			}
			fp.Close()
		}(c.fp)

		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, os.ModePerm); err != nil {
			return c.write(nil, err)
		}

		fp, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
		if err != nil {
			return c.write(nil, err)
		}
		c.createSymlink(t, path)

		c.path = path
		c.fp = fp
	}

	return c.write(b, nil)
}

// Path returns the current writing file path.
func (c *RotateWriter) Path() string {
	c.mux.Lock()
	defer c.mux.Unlock()
	return c.path
}

func (c *RotateWriter) createSymlink(t time.Time, path string) {
	if c.symlink == nil {
		return
	}

	symlink := c.symlink.FormatString(t)
	if symlink == path {
		c.log.Error("Can't create symlink. Already file exists.")
		return // ignore error
	}

	if _, err := os.Stat(symlink); err == nil {
		if err := os.Remove(symlink); err != nil {
			c.log.Error(err)
			return // ignore error
		}
	}

	if err := os.Symlink(path, symlink); err != nil {
		c.log.Error(err)
		return // ignore error
	}
}

// Close closes file.
func (c *RotateWriter) Close() error {
	c.mux.Lock()
	defer c.mux.Unlock()

	return c.fp.Close()
}

func (c *RotateWriter) write(b []byte, err error) (int, error) {
	if err != nil {
		c.log.Error(err)
		return 0, err
	}

	c.log.Write(b)
	return c.fp.Write(b)
}
