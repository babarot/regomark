package judge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/babarot/regomark/internal/input"
	"github.com/babarot/regomark/internal/policy"
)

// Judge runs a command for each question.
type Judge struct {
	// Command is the command and its arguments, run without a shell in Dir.
	Command []string
	Dir     string
	// Parallel is how many commands run at once, and Timeout the limit for
	// each.
	Parallel int
	Timeout  time.Duration
	// CacheDir holds the answers already given; empty means no cache.
	CacheDir string
}

// Stats counts what a run did.
type Stats struct {
	Questions int // questions asked about
	Cached    int // answered from the cache
	Asked     int // answered by running the command
	Failed    int // not answered: the command failed or its answer was not one
	// CacheErr is the first error writing to the cache, which does not stop
	// the run.
	CacheErr error
}

// Run asks every question about the documents in sources and returns the
// judgments in the order of the questions. A question the command cannot
// answer gets a judgment saying why; Run fails only when a question does
// not fit its source.
func (j *Judge) Run(ctx context.Context, qs []policy.Question, sources map[string][]byte) ([]input.Judgment, Stats, error) {
	type job struct {
		prompt string
		sent   []int
	}
	jobs := make([]job, len(qs))
	for i, q := range qs {
		src, ok := sources[q.Path]
		if !ok {
			return nil, Stats{}, fmt.Errorf("no source for %s", q.Path)
		}
		prompt, sent, err := Prompt(q, src)
		if err != nil {
			return nil, Stats{}, err
		}
		jobs[i] = job{prompt, sent}
	}

	judgments := make([]input.Judgment, len(qs))
	stats := Stats{Questions: len(qs)}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, max(j.Parallel, 1))
	for i, q := range qs {
		judgments[i] = input.Judgment{ID: q.ID, Path: q.Path, Line: q.Line}
		key := cacheKey(Key(jobs[i].prompt), j.Command)
		if a, ok := j.cached(key, jobs[i].sent); ok {
			setAnswer(&judgments[i], a)
			stats.Cached++
			continue
		}
		wg.Add(1)
		go func(i int, key string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			a, status, err := j.ask(ctx, jobs[i].prompt, jobs[i].sent)
			var cacheErr error
			if err == nil {
				setAnswer(&judgments[i], a)
				cacheErr = j.store(key, a)
			} else {
				judgments[i].Status = status
				judgments[i].Error = err.Error()
			}
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				stats.Asked++
			} else {
				stats.Failed++
			}
			if stats.CacheErr == nil {
				stats.CacheErr = cacheErr
			}
		}(i, key)
	}
	wg.Wait()
	return judgments, stats, nil
}

func setAnswer(j *input.Judgment, a Answer) {
	j.Status, j.Verdict, j.Reason, j.EvidenceLine = input.StatusOK, a.Verdict, a.Reason, a.EvidenceLine
}

// ask runs the command for one prompt and returns its answer, or the status
// and the reason it gave none.
func (j *Judge) ask(ctx context.Context, prompt string, sent []int) (Answer, string, error) {
	ctx, cancel := context.WithTimeout(ctx, j.Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, j.Command[0], j.Command[1:]...)
	cmd.Dir = j.Dir
	cmd.Stdin = strings.NewReader(prompt)
	cmd.Env = append(os.Environ(), "REGOMARK_ANSWER_SCHEMA="+AnswerSchema)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	killGroup(cmd)
	// A child that keeps the output open must not hold the run past the
	// timeout
	cmd.WaitDelay = 5 * time.Second
	err := cmd.Run()
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return Answer{}, input.StatusFailed, fmt.Errorf("timed out after %s", j.Timeout)
	case err != nil:
		msg := err.Error()
		if s := tail(stderr.String(), 500); s != "" {
			msg += ": " + s
		}
		return Answer{}, input.StatusFailed, errors.New(msg)
	}
	raw, err := decodeOutput(stdout.Bytes())
	if errors.Is(err, errFailed) {
		return Answer{}, input.StatusFailed, err
	}
	if err != nil {
		return Answer{}, input.StatusInvalidAnswer, err
	}
	a, err := checkAnswer(raw, sent)
	if err != nil {
		return Answer{}, input.StatusInvalidAnswer, err
	}
	return a, "", nil
}

// cached returns the answer in the cache, checked again; a missing or broken
// entry is a miss.
func (j *Judge) cached(key string, sent []int) (Answer, bool) {
	if j.CacheDir == "" {
		return Answer{}, false
	}
	b, err := os.ReadFile(filepath.Join(j.CacheDir, key+".json"))
	if err != nil {
		return Answer{}, false
	}
	a, err := checkAnswer(b, sent)
	return a, err == nil
}

// store writes an answer to the cache through a temporary file, so that a
// reader never sees half of one.
func (j *Judge) store(key string, a Answer) error {
	if j.CacheDir == "" {
		return nil
	}
	if err := os.MkdirAll(j.CacheDir, 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(a)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(j.CacheDir, key+".*.tmp")
	if err != nil {
		return err
	}
	_, werr := f.Write(append(b, '\n'))
	cerr := f.Close()
	if err := errors.Join(werr, cerr); err != nil {
		_ = os.Remove(f.Name())
		return err
	}
	if err := os.Rename(f.Name(), filepath.Join(j.CacheDir, key+".json")); err != nil {
		_ = os.Remove(f.Name())
		return err
	}
	return nil
}
