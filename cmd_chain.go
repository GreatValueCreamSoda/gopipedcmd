package gopipedcmd


import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

type Cmd struct {
	Target string
	Args   []string
}

func Run(cmd ...Cmd) error {
	ctx, cancel := context.WithCancel(context.Background())
	return runner(ctx, cancel, cmd...)
}

func RunWithDeadline(time time.Duration, cmd ...Cmd) error {
	ctx, cancel := context.WithTimeout(context.Background(), time)
	return runner(ctx, cancel, cmd...)
}

func RunWithContext(ctx context.Context, cancel context.CancelFunc, cmd ...Cmd) error {
	return runner(ctx, cancel, cmd...)
}

func runner(ctx context.Context, cancel context.CancelFunc, cmd ...Cmd) error {
	var (
		stderr    = make([]bytes.Buffer, len(cmd))
		errCh     = make(chan error, len(cmd))
		wg        sync.WaitGroup
		cmds, err = parseCmds(ctx, cmd...)
	)

	defer cancel()
	defer close(errCh)

	if err != nil {
		return err
	}

	for idx := 0; idx < len(cmds)-1; idx++ {
		cmds[idx+1].Stdin, cmds[idx].Stdout = io.Pipe()
		cmds[idx].Stderr = &stderr[idx]
	}

	cmds[len(cmds)-1].Stderr = &stderr[len(cmds)-1]

	for idx := 0; idx < len(cmds); idx++ {
		wg.Add(1)
		go execCmd(cmds[idx], &wg, errCh)
	}

	go func() {
		wg.Wait()
		cancel()
	}()

	select {
	case err := <-errCh:
		cancel()
		wg.Wait()
		return err
	case <-ctx.Done():
		if err := context.Background().Err(); err == context.DeadlineExceeded {
			return err
		}
	}
	return nil
}

func execCmd(cmd *exec.Cmd, wg *sync.WaitGroup, errCh chan error) {
	defer wg.Done()
	if cmd.Stdout != nil {
		defer cmd.Stdout.(*io.PipeWriter).Close()
	}
	if err := cmd.Start(); err != nil {
		errCh <- fmt.Errorf("failed to start command: %w", err)
		return
	}
	if err := cmd.Wait(); err != nil {
		errCh <- fmt.Errorf("command failed: %s", cmd.Stderr.(*bytes.Buffer).String())
		return
	}
}

func parseCmds(ctx context.Context, cmd ...Cmd) ([]*exec.Cmd, error) {
	var cmds []*exec.Cmd = make([]*exec.Cmd, len(cmd))
	var err error
	for idx := 0; idx < len(cmd); idx++ {
		cmds[idx], err = cmd[idx].createCmd(ctx)
		if err != nil {
			return nil, err
		}
	}
	return cmds, nil
}

func (cmd *Cmd) createCmd(ctx context.Context) (*exec.Cmd, error) {
	return exec.CommandContext(ctx, cmd.Target, cmd.Args...), nil
}
