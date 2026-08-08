package main

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/store/pgstore"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/hive"
	"github.com/transpara-ai/hive/pkg/hive/factoryv1"
	"github.com/transpara-ai/hive/pkg/social"
	"github.com/transpara-ai/work"
)

func main() {
	addr := flag.String("addr", envOrDefault("HIVE_OPS_API_ADDR", "127.0.0.1:8083"), "listen address")
	apiKey := flag.String("api-key", envOrDefault("HIVE_OPS_API_KEY", "dev"), "bearer token for Site operator projection reads")
	limit := flag.Int("limit", 50, "maximum records per projection section")
	catalog := flag.String("catalog", envOrDefault("HIVE_OPS_CATALOG", ""), "custom YAML model catalog for operator projection")
	catalogReloadInterval := flag.Duration("catalog-reload-interval", durationEnvOrDefault("HIVE_OPS_CATALOG_RELOAD_INTERVAL", 0), "model catalog reload interval; 0 disables runtime reload")
	flag.Parse()

	dsn := envOrDefault("HIVE_OPS_DATABASE_URL", os.Getenv("DATABASE_URL"))
	if dsn == "" {
		dsn = "postgres://hive:hive@localhost:5432/hive?sslmode=disable"
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("open postgres: %v", err)
	}
	defer pool.Close()

	store, err := pgstore.NewPostgresStoreFromPool(ctx, pool)
	if err != nil {
		log.Fatalf("open eventgraph store: %v", err)
	}
	defer store.Close()

	registerOpsAPIEventTypes()

	modelSelectionManager, err := hive.NewOperatorModelSelectionManager(*catalog, types.Now().Value(), *catalogReloadInterval > 0)
	if err != nil {
		log.Fatalf("load model catalog: %v", err)
	}
	if *catalogReloadInterval > 0 && *catalog != "" {
		go runCatalogReloadLoop(ctx, modelSelectionManager, *catalogReloadInterval)
	}

	opts, writeMode := opsWriterOptions()
	factoryV1Option, factoryV1Service, factoryV1Mode := factoryV1OpsOption(store)
	if factoryV1Option != nil {
		opts = append(opts, factoryV1Option)
		writeMode += ",factory-v1:" + factoryV1Mode
	} else {
		writeMode += ",factory-v1:" + factoryV1Mode
	}
	opts = append(opts, hive.WithOperatorProjectionModelSelectionSource(modelSelectionManager.Snapshot))
	var factoryProjection hive.FactoryProjectionBuilder
	if factoryV1Service != nil {
		factoryProjection = factoryV1Service.Projector.Build
	}
	runtimeSource := hive.FactoryRuntimeClient{
		Endpoint: os.Getenv("HIVE_FACTORY_V1_RUNTIME_URL"),
		APIKey:   os.Getenv("HIVE_FACTORY_V1_RUNTIME_API_KEY"),
	}
	missionControl, err := hive.NewCivilizationMissionControlProjector(store, hive.MissionControlProjectorConfig{
		FactoryProjection: factoryProjection,
		ModelSelection:    modelSelectionManager.Snapshot,
		Runtime:           runtimeSource,
		PageSize:          *limit,
	})
	if err != nil {
		log.Fatalf("configure Civilization Mission Control: %v", err)
	}
	opts = append(opts, hive.WithMissionControlProjection(missionControl))
	handler := hive.NewOperatorProjectionServer(store, *apiKey, *limit, opts...)
	modelSelection := modelSelectionManager.Snapshot()

	authMode := "disabled"
	if *apiKey != "" {
		authMode = "bearer"
	}
	fmt.Printf("hive ops api listening on %s (auth=%s, limit=%d, writes=%s, model_catalog=%s, reload=%s)\n", *addr, authMode, *limit, writeMode, modelSelection.CatalogSource, modelSelection.ReloadMode)
	if err := http.ListenAndServe(*addr, handler); err != nil {
		log.Fatalf("listen: %v", err)
	}
}

func factoryV1OpsOption(eventStore store.Store) (hive.OperatorServerOption, *hive.FactoryV1OperatorService, string) {
	if os.Getenv("HIVE_FACTORY_V1_ENABLED") != "true" {
		return nil, nil, "disabled"
	}
	humanValue := os.Getenv("HIVE_OPS_HUMAN_ACTOR")
	credentialKeyID := os.Getenv("HIVE_FACTORY_V1_CREDENTIAL_KEY_ID")
	if humanValue == "" || credentialKeyID == "" {
		log.Printf("factory v1 routes disabled: HIVE_OPS_HUMAN_ACTOR and HIVE_FACTORY_V1_CREDENTIAL_KEY_ID are required")
		return nil, nil, "misconfigured"
	}
	humanID, err := types.NewActorID(humanValue)
	if err != nil {
		log.Printf("factory v1 routes disabled: invalid Human actor: %v", err)
		return nil, nil, "misconfigured"
	}
	registry := event.DefaultRegistry()
	hive.RegisterWithRegistry(registry)
	work.RegisterWithRegistry(registry)
	factory := event.NewEventFactory(registry)
	seed := "signer:" + humanID.Value()
	if key := os.Getenv("HIVE_OPS_SIGNING_KEY"); key != "" {
		seed = key
	}
	signer := newOpsSigner(seed)
	conversation := types.MustConversationID("conv_hive_factory_v1_ops_api")
	graph, err := hive.NewFactoryV1EventGraphStore(eventStore, factory, signer, humanID, conversation)
	if err != nil {
		log.Printf("factory v1 routes disabled: EventGraph adapter: %v", err)
		return nil, nil, "misconfigured"
	}
	workStore, err := hive.NewFactoryV1WorkStore(eventStore, factory, signer, humanID, conversation)
	if err != nil {
		log.Printf("factory v1 routes disabled: Work adapter: %v", err)
		return nil, nil, "misconfigured"
	}
	clock := factoryv1.WallClock{}
	intake, err := factoryv1.NewIntake(graph, workStore, clock)
	if err != nil {
		log.Printf("factory v1 routes disabled: intake: %v", err)
		return nil, nil, "misconfigured"
	}
	if err := intake.ReplayAndRepair(context.Background()); err != nil {
		log.Printf("factory v1 routes disabled: recovery: %v", err)
		return nil, nil, "recovery-failed"
	}
	recoveryGeneration, _ := strconv.Atoi(os.Getenv("HIVE_FACTORY_V1_RECOVERY_GENERATION"))
	serviceProjection := factoryv1.ServiceProjection{
		ServiceID: "hive-factory-v1", InstanceID: envOrDefault("HIVE_FACTORY_V1_INSTANCE_ID", "hive-ops-api"),
		RecoveryGeneration: recoveryGeneration, StartedAt: time.Now().UTC(), Healthy: true,
	}
	projector, err := factoryv1.NewProjector(graph, workStore, clock, serviceProjection)
	if err != nil {
		log.Printf("factory v1 routes disabled: projector: %v", err)
		return nil, nil, "misconfigured"
	}
	service, err := hive.NewFactoryV1OperatorService(intake, projector, graph, clock, humanID.Value(), credentialKeyID)
	if err != nil {
		log.Printf("factory v1 routes disabled: operator service: %v", err)
		return nil, nil, "misconfigured"
	}
	return hive.WithOperatorFactoryV1(service), service, "enabled"
}

func registerOpsAPIEventTypes() {
	hive.RegisterEventTypes()
	social.RegisterEventTypes()
	work.RegisterEventTypes()
	event.SetFallbackUnmarshaler(event.RawFallback)
}

func runCatalogReloadLoop(ctx context.Context, manager *hive.OperatorModelSelectionManager, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			changed, err := manager.ReloadIfChanged(types.Now().Value())
			if err != nil {
				log.Printf("model catalog reload failed: %v", err)
				continue
			}
			if changed {
				snapshot := manager.Snapshot()
				log.Printf("model catalog reloaded: %s", snapshot.CatalogSource)
			}
		}
	}
}

// opsWriterOptions provisions optional operator write routes from the
// environment. Writers are enabled only when HIVE_OPS_HUMAN_ACTOR is set;
// otherwise the server stays strictly read-only (today's behavior), so an
// unconfigured ops-api must not fail. The graph is still only ever written by
// hive — this process — never by Site.
//
//   - HIVE_OPS_HUMAN_ACTOR : the human actor id recorded as the approver/signer
//     of operator decisions and launch requests. Required to enable write paths.
//   - HIVE_OPS_SIGNING_KEY : optional explicit signing seed. When unset, the
//     signer is derived deterministically from the human actor id, matching the
//     hive runtime identity scheme (sha256("signer:"+id)).
func opsWriterOptions() ([]hive.OperatorServerOption, string) {
	human := os.Getenv("HIVE_OPS_HUMAN_ACTOR")
	if human == "" {
		return nil, "read-only"
	}
	humanID, err := types.NewActorID(human)
	if err != nil {
		log.Printf("HIVE_OPS_HUMAN_ACTOR invalid (%v); operator writes disabled, server read-only", err)
		return nil, "read-only"
	}

	registry := event.DefaultRegistry()
	hive.RegisterWithRegistry(registry)
	factory := event.NewEventFactory(registry)

	seed := "signer:" + humanID.Value()
	if key := os.Getenv("HIVE_OPS_SIGNING_KEY"); key != "" {
		seed = key
	}
	signer := newOpsSigner(seed)
	conv := types.MustConversationID("conv_hive_ops_api")

	return []hive.OperatorServerOption{
		hive.WithOperatorDecisionWriter(factory, signer, humanID, conv),
		hive.WithOperatorRunLaunchWriter(factory, signer, humanID, conv),
		hive.WithOperatorModelRolePolicyWriter(factory, signer, humanID, conv),
	}, "enabled"
}

// opsSigner is a deterministic Ed25519 signer for the ops-api decision writer,
// matching the hive runtime's signer scheme.
type opsSigner struct {
	key ed25519.PrivateKey
}

func (s *opsSigner) Sign(data []byte) (types.Signature, error) {
	sig := ed25519.Sign(s.key, data)
	return types.NewSignature(sig)
}

func newOpsSigner(seed string) *opsSigner {
	h := sha256.Sum256([]byte(seed))
	return &opsSigner{key: ed25519.NewKeyFromSeed(h[:])}
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func durationEnvOrDefault(name string, fallback time.Duration) time.Duration {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		log.Printf("%s invalid duration %q: %v; using %s", name, value, err, fallback)
		return fallback
	}
	return d
}
