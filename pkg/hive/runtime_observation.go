package hive

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/transpara-ai/hive/pkg/loop"
)

const runtimeObservationSchema = "hive-runtime-observation/v1"
const runtimeObservationMaxAge = 15 * time.Second

// RuntimeAgentObservation describes a loop owned by this daemon process.
// It carries no task, approval, or durable event authority.
type RuntimeAgentObservation struct {
	ActorID string            `json:"actor_id"`
	Role    string            `json:"role"`
	State   loop.RuntimeState `json:"state"`
}

type RuntimeObservation struct {
	Schema     string                    `json:"schema"`
	ObservedAt time.Time                 `json:"observed_at"`
	Agents     []RuntimeAgentObservation `json:"agents"`
}

func (r *Runtime) observeAgentRuntime(actorID, role string) func(loop.RuntimeState) {
	return func(state loop.RuntimeState) {
		r.runtimeObservationMu.Lock()
		defer r.runtimeObservationMu.Unlock()
		if r.runtimeObservations == nil {
			r.runtimeObservations = make(map[string]RuntimeAgentObservation)
		}
		r.runtimeObservations[actorID] = RuntimeAgentObservation{ActorID: actorID, Role: role, State: state}
	}
}

func (r *Runtime) runtimeObservation(now time.Time) RuntimeObservation {
	r.runtimeObservationMu.Lock()
	defer r.runtimeObservationMu.Unlock()
	observation := RuntimeObservation{Schema: runtimeObservationSchema, ObservedAt: now, Agents: []RuntimeAgentObservation{}}
	for _, agent := range r.runtimeObservations {
		observation.Agents = append(observation.Agents, agent)
	}
	sort.Slice(observation.Agents, func(i, j int) bool { return observation.Agents[i].ActorID < observation.Agents[j].ActorID })
	return observation
}

// The directory is daemon-writable and mounted read-only in the ops API. Atomic
// replacement avoids partial reads; freshness expires after an abrupt process exit.
func writeRuntimeObservation(path string, observation RuntimeObservation) error {
	body, err := json.Marshal(observation)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".runtime-observation-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(body); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

func (r *Runtime) startRuntimeObservationWriter() func() {
	path := os.Getenv("HIVE_RUNTIME_SNAPSHOT_FILE")
	if path == "" || r.isolateRunTasks {
		return func() {}
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			if err := writeRuntimeObservation(path, r.runtimeObservation(time.Now().UTC())); err != nil {
				log.Printf("runtime observation unavailable: %v", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return func() {
		cancel()
		<-done
		if err := writeRuntimeObservation(path, r.runtimeObservation(time.Now().UTC())); err != nil {
			log.Printf("final runtime observation unavailable: %v", err)
		}
	}
}

func readRuntimeObservation(path string, now time.Time) (RuntimeObservation, error) {
	file, err := os.Open(path)
	if err != nil {
		return RuntimeObservation{}, err
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, 1024*1024+1))
	if err != nil || len(body) > 1024*1024 {
		return RuntimeObservation{}, errors.New("runtime observation exceeds limit or cannot be read")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var observation RuntimeObservation
	if err := decoder.Decode(&observation); err != nil {
		return observation, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return observation, errors.New("runtime observation has trailing content")
	}
	if observation.Schema != runtimeObservationSchema || observation.ObservedAt.IsZero() || observation.ObservedAt.After(now.Add(5*time.Second)) || now.Sub(observation.ObservedAt) > runtimeObservationMaxAge {
		return observation, errors.New("runtime observation is invalid or expired")
	}
	seen := map[string]bool{}
	for _, agent := range observation.Agents {
		if agent.ActorID == "" || agent.Role == "" || seen[agent.ActorID] {
			return observation, errors.New("runtime identity is missing or duplicated")
		}
		seen[agent.ActorID] = true
		switch agent.State {
		case loop.RuntimeWorking, loop.RuntimeWaiting, loop.RuntimeBudgetWait, loop.RuntimeStopped:
		default:
			return observation, errors.New("runtime state is unknown")
		}
	}
	return observation, nil
}

func applyRuntimeObservations(rows []RoleAgentRow, path string, now time.Time) ServiceHealth {
	observation, err := readRuntimeObservation(path, now)
	if err != nil {
		mark := missionUnavailableMark("hive_runtime", now, "Daemon runtime observation is missing, invalid, or older than 15 seconds.")
		for i := range rows {
			rows[i].Running = missionMarked(nil, mark)
		}
		return ServiceHealth{ServiceID: "hive_runtime", Label: "Hive daemon", OperationalStatus: "unavailable", Detail: mark.Reason, Mark: mark}
	}
	mark := NewEvidenceMark(FreshnessCurrent, BasisProjectedOnly, "hive_runtime", observation.ObservedAt, now, nil, "Live daemon loop observation; not durable evidence or task authority.")
	actors := map[string]RuntimeAgentObservation{}
	counts := map[string]int{}
	for _, agent := range observation.Agents {
		actors[agent.ActorID] = agent
		if agent.State != loop.RuntimeStopped {
			counts[agent.Role]++
		}
	}
	for i := range rows {
		row := &rows[i]
		if row.ActorID == "" {
			row.Running = missionMarked(counts[row.Role], mark)
			continue
		}
		agent, ok := actors[row.ActorID]
		if !ok {
			row.Running = missionMarked(false, mark)
			row.Status = missionMarked("not in this runtime", mark)
			continue
		}
		if agent.Role != row.Role {
			row.Running = missionMarked(nil, missionUnavailableMark("hive_runtime", now, "Runtime role does not match durable actor identity."))
			continue
		}
		row.Running = missionMarked(agent.State != loop.RuntimeStopped, mark)
		row.Status = missionMarked(string(agent.State), mark)
	}
	return ServiceHealth{ServiceID: "hive_runtime", Label: "Hive daemon", OperationalStatus: "healthy", Detail: "Live loop states, refreshed every three seconds. Idle agents are running and waiting for work.", Mark: mark}
}
