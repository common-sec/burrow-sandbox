package consent

import (
	"io"
	"os"
	"runtime"
)

// ControllingTerminal opens the process's controlling terminal, never stdin, so a pipe
// (`yes | burrow ...`) cannot answer. On Windows it is the console (CONIN$ and CONOUT$).
type ControllingTerminal struct{}

// Open opens the terminal, or fails when the process has none.
func (ControllingTerminal) Open() (io.ReadWriteCloser, error) {
	if runtime.GOOS == "windows" {
		in, err := os.Open("CONIN$")
		if err != nil {
			return nil, err
		}
		out, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0)
		if err != nil {
			_ = in.Close() // already failing; the open error is the one to report
			return nil, err
		}
		return console{in, out}, nil
	}
	return os.OpenFile("/dev/tty", os.O_RDWR, 0)
}

type console struct{ in, out *os.File }

func (c console) Read(p []byte) (int, error)  { return c.in.Read(p) }
func (c console) Write(p []byte) (int, error) { return c.out.Write(p) }
func (c console) Close() error {
	err := c.in.Close()
	if err2 := c.out.Close(); err == nil {
		err = err2
	}
	return err
}
