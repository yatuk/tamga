//go:build race

package proxy

// raceDetector says the tests were built with -race, under which a scan takes
// many times longer than in the binary as shipped.
const raceDetector = true
