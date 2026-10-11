//go:build race

package scanner

// raceDetector says the tests were built with -race. The detector makes a
// regexp search many times slower, so a bound on wall time that holds for the
// shipped binary does not hold for the instrumented one (see searchSlack).
const raceDetector = true
