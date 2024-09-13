package gopipedcmd

import (
	"bytes"
	"errors"
	"io"
	"os/exec"
	"sync"
)

func RunPipedCommands(cmds ...*exec.Cmd) error {
	var readers []*io.PipeReader = make([]*io.PipeReader, len(cmds)-1)
	var writers []*io.PipeWriter = make([]*io.PipeWriter, len(cmds)-1)
	var stderrs []bytes.Buffer = make([]bytes.Buffer, len(cmds))
	var wg sync.WaitGroup
	var mt sync.Mutex
	var crashErr error = nil

	for idx := 0; idx < len(cmds)-1; idx++ {
		readers[idx], writers[idx] = io.Pipe()
		cmds[idx].Stdout = writers[idx]
		cmds[idx+1].Stdin = readers[idx]
		cmds[idx].Stderr = &stderrs[idx]
	}

	cmds[len(cmds)-1].Stderr = &stderrs[len(cmds)-1]

	defer func() {
		for idx := 0; idx < len(cmds)-1; idx++ {
			readers[idx].Close()
		}
	}()

	threadWatcher := func(idx int) {
		for {
			err := cmds[idx].Wait()
			if err == nil {
				if idx != len(cmds)-1 {
					writers[idx].Close()
				}
				wg.Done()
				return
			}
			if !mt.TryLock() {
				wg.Done()
				return
			}
			crashErr = errors.New(stderrs[idx].String())
			for i := range cmds {
				cmds[i].Process.Kill()
			}
			wg.Done()
		}
	}

	for idx := range cmds {
		err := cmds[idx].Start()
		if err != nil {
			return err
		}
		wg.Add(1)
		go threadWatcher(idx)
	}
	wg.Wait()
	return crashErr
}
