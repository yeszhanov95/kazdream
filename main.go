package main

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

type job struct {
	gid   int // id of executing goroutine
	url   string
	code  int
	size  int64
	rtime time.Duration
	err   error
}

func main() {
	done := make(chan struct{})

	sigs := make(chan os.Signal)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigs
		fmt.Println()
		fmt.Println(sig)
		done <- struct{}{}
	}()

	// initialize map and fill
	m := make(map[int]int) // key - goroutine id : val - count of jobs done
	for gid := 0; gid < runtime.GOMAXPROCS(0); gid++ {
		m[gid] = 0
	}

	urlsChan := readUrls(bufio.NewReader(os.Stdin))
	doneJobsChan := processJobs(urlsChan)

	go func() {
		for job := range doneJobsChan {
			m[job.gid]++
			if job.err != nil {
				fmt.Printf("%s\n", job.err.Error())
			} else {
				fmt.Printf("%s;%d;%d;%dms\n", job.url, job.code, job.size, job.rtime.Milliseconds())
			}
		}
		done <- struct{}{}
	}()

	<-done

	for gid, cnt := range m {
		fmt.Printf("%d: %d\n", gid, cnt)
	}
}

func processJobs(urlsChan <-chan string) <-chan job {
	doneJobsChan := make(chan job)

	go func() {
		procsWg := &sync.WaitGroup{}
		urlsWg := &sync.WaitGroup{}

		for gid := 0; gid < runtime.GOMAXPROCS(0); gid++ {
			procsWg.Add(1)

			go func(gid int) {
				defer procsWg.Done()

				for url := range urlsChan {
					urlsWg.Add(1)
					go fetchUrl(url, gid, doneJobsChan, urlsWg)
				}
			}(gid)
		}

		urlsWg.Wait()
		procsWg.Wait()

		close(doneJobsChan)
	}()

	return doneJobsChan
}

func fetchUrl(url string, gid int, doneJobs chan<- job, wg *sync.WaitGroup) {
	defer wg.Done()

	doneJob := job{gid: gid, url: url}
	now := time.Now()
	resp, err := http.Get(url)
	if err != nil {
		doneJob.err = err
		doneJobs <- doneJob
		return
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		doneJob.err = err
	}

	doneJob.rtime = time.Since(now)
	doneJob.code = resp.StatusCode
	doneJob.size = int64(len(body))

	doneJobs <- doneJob
}

func readUrls(reader *bufio.Reader) <-chan string {
	urlsChan := make(chan string)

	go func() {
		for {
			url, err := reader.ReadString('\n')
			if err == io.EOF {
				break
			}
			if err != nil {
				fmt.Printf("%s\n", err.Error())
				continue
			}
			urlsChan <- strings.TrimSpace(url)
		}

		close(urlsChan)
	}()

	return urlsChan
}
