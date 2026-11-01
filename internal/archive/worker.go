package archive

import "sync"

func worker(
	jobs <-chan Job,
	results chan<- Result,
	wg *sync.WaitGroup,
)
