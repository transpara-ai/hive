package runner

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// DailyBudget tracks accumulated spend for today using a file in loop/.
// Format: loop/budget-YYYYMMDD.txt — one float64 per line, summed.
type DailyBudget struct {
	dir  string // directory containing the loop/ folder (hiveDir)
	date string // YYYYMMDD for today
}

// NewDailyBudget creates a DailyBudget rooted at hiveDir.
func NewDailyBudget(hiveDir string) *DailyBudget {
	return &DailyBudget{
		dir:  hiveDir,
		date: time.Now().Format("20060102"),
	}
}

func (d *DailyBudget) path() string {
	return filepath.Join(d.dir, "loop", fmt.Sprintf("budget-%s.txt", d.date))
}

// Record appends amount to today's budget file.
func (d *DailyBudget) Record(amount float64) {
	p := d.path()
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		log.Printf("[budget] mkdir error: %v", err)
		return
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("[budget] open error: %v", err)
		return
	}
	defer f.Close()
	if _, err := fmt.Fprintf(f, "%f\n", amount); err != nil {
		log.Printf("[budget] write error: %v", err)
	}
}

// Spent returns the sum of all recorded amounts for today.
func (d *DailyBudget) Spent() float64 {
	data, err := os.ReadFile(d.path())
	if err != nil {
		return 0 // file not yet created is fine
	}
	var total float64
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		v, err := strconv.ParseFloat(line, 64)
		if err != nil {
			log.Printf("[budget] parse error for %q: %v", line, err)
			continue
		}
		total += v
	}
	return total
}

// Remaining returns ceiling minus today's spend. Returns 0 if already over.
func (d *DailyBudget) Remaining(ceiling float64) float64 {
	r := ceiling - d.Spent()
	if r < 0 {
		return 0
	}
	return r
}
