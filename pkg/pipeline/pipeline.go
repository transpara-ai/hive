// Package pipeline orchestrates the product build pipeline.
package pipeline

import (
	"context"
	"fmt"
	"strings"

	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/intelligence"
	"github.com/lovyou-ai/eventgraph/go/pkg/store"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"

	"github.com/lovyou-ai/hive/pkg/roles"
	"github.com/lovyou-ai/hive/pkg/workspace"
)

// Phase represents a stage in the product pipeline.
type Phase string

const (
	PhaseResearch  Phase = "research"
	PhaseDesign    Phase = "design"
	PhaseBuild     Phase = "build"
	PhaseReview    Phase = "review"
	PhaseTest      Phase = "test"
	PhaseIntegrate Phase = "integrate"
)

// ProductInput describes how a product idea enters the hive.
type ProductInput struct {
	URL         string // Read from URL (Substack post, docs, etc.)
	Description string // Natural language description
	SpecFile    string // Path to a Code Graph spec file
}

// Pipeline orchestrates agents through the product build phases.
type Pipeline struct {
	store    store.Store
	provider intelligence.Provider
	ws       *workspace.Workspace

	cto      *roles.Agent
	guardian *roles.Agent
	agents   map[roles.Role]*roles.Agent
}

// Config for creating a new pipeline.
type Config struct {
	Store      store.Store
	Provider   intelligence.Provider
	WorkDir    string // Root directory for generated products
}

// New creates a pipeline and bootstraps the CTO and Guardian.
func New(ctx context.Context, cfg Config) (*Pipeline, error) {
	ws, err := workspace.New(cfg.WorkDir)
	if err != nil {
		return nil, fmt.Errorf("workspace: %w", err)
	}

	p := &Pipeline{
		store:    cfg.Store,
		provider: cfg.Provider,
		ws:       ws,
		agents:   make(map[roles.Role]*roles.Agent),
	}

	// Bootstrap CTO first — architectural oversight
	cto, err := roles.NewAgent(ctx, roles.AgentConfig{
		Role:     roles.RoleCTO,
		Name:     "cto",
		Store:    cfg.Store,
		Provider: cfg.Provider,
	})
	if err != nil {
		return nil, fmt.Errorf("bootstrap CTO: %w", err)
	}
	p.cto = cto
	p.agents[roles.RoleCTO] = cto

	// Bootstrap Guardian — independent integrity monitor
	guardian, err := roles.NewAgent(ctx, roles.AgentConfig{
		Role:     roles.RoleGuardian,
		Name:     "guardian",
		Store:    cfg.Store,
		Provider: cfg.Provider,
	})
	if err != nil {
		return nil, fmt.Errorf("bootstrap Guardian: %w", err)
	}
	p.guardian = guardian
	p.agents[roles.RoleGuardian] = guardian

	return p, nil
}

// ensureAgent creates an agent of the given role if it doesn't exist yet.
func (p *Pipeline) ensureAgent(ctx context.Context, role roles.Role, name string) (*roles.Agent, error) {
	if agent, ok := p.agents[role]; ok {
		return agent, nil
	}
	agent, err := roles.NewAgent(ctx, roles.AgentConfig{
		Role:     role,
		Name:     name,
		Store:    p.store,
		Provider: p.provider,
	})
	if err != nil {
		return nil, err
	}
	p.agents[role] = agent
	return agent, nil
}

// Run executes the full product pipeline for a given input.
func (p *Pipeline) Run(ctx context.Context, input ProductInput) error {
	fmt.Println("═══ Phase 1: Research ═══")
	spec, err := p.research(ctx, input)
	if err != nil {
		return fmt.Errorf("research: %w", err)
	}

	fmt.Println("═══ Phase 2: Design ═══")
	design, err := p.design(ctx, spec)
	if err != nil {
		return fmt.Errorf("design: %w", err)
	}

	fmt.Println("═══ Phase 3: Build ═══")
	code, err := p.build(ctx, design)
	if err != nil {
		return fmt.Errorf("build: %w", err)
	}

	fmt.Println("═══ Phase 4: Review ═══")
	err = p.review(ctx, code, design)
	if err != nil {
		return fmt.Errorf("review: %w", err)
	}

	fmt.Println("═══ Phase 5: Test ═══")
	err = p.test(ctx, code)
	if err != nil {
		return fmt.Errorf("test: %w", err)
	}

	fmt.Println("═══ Phase 6: Integrate ═══")
	err = p.integrate(ctx)
	if err != nil {
		return fmt.Errorf("integrate: %w", err)
	}

	fmt.Println("═══ Pipeline Complete ═══")
	return nil
}

// research gathers information about the product idea.
func (p *Pipeline) research(ctx context.Context, input ProductInput) (string, error) {
	var spec string

	if input.SpecFile != "" {
		// Read the spec file directly
		content, err := p.ws.ReadFile(input.SpecFile)
		if err != nil {
			return "", fmt.Errorf("read spec: %w", err)
		}
		spec = content
	} else {
		researcher, err := p.ensureAgent(ctx, roles.RoleResearcher, "researcher")
		if err != nil {
			return "", err
		}

		if input.URL != "" {
			_, evaluation, err := researcher.Runtime.Research(ctx, input.URL,
				"extract the product idea, key entities, features, and requirements. Output in Code Graph vocabulary where possible.")
			if err != nil {
				return "", fmt.Errorf("research URL: %w", err)
			}
			spec = evaluation
		} else if input.Description != "" {
			_, evaluation, err := researcher.Runtime.Evaluate(ctx, "product_idea", input.Description)
			if err != nil {
				return "", fmt.Errorf("evaluate idea: %w", err)
			}
			spec = evaluation
		}
	}

	// CTO evaluates feasibility
	_, ctoEval, err := p.cto.Runtime.Evaluate(ctx, "feasibility",
		fmt.Sprintf("Evaluate this product idea for feasibility. What agents are needed? What's the build sequence? Key risks?\n\n%s", spec))
	if err != nil {
		return "", fmt.Errorf("CTO evaluate: %w", err)
	}

	fmt.Printf("CTO Assessment:\n%s\n", ctoEval)
	return spec, nil
}

// design creates a full Code Graph spec from the product idea.
func (p *Pipeline) design(ctx context.Context, spec string) (string, error) {
	architect, err := p.ensureAgent(ctx, roles.RoleArchitect, "architect")
	if err != nil {
		return "", err
	}

	prompt := fmt.Sprintf("%s\n\nDesign the full system architecture. Output a complete Code Graph spec with entities, states, views, layouts, queries, commands, triggers, and constraints.\n\n%s",
		roles.SystemPrompt(roles.RoleArchitect), spec)

	_, design, err := architect.Runtime.Evaluate(ctx, "architecture", prompt)
	if err != nil {
		return "", fmt.Errorf("architect design: %w", err)
	}

	// CTO reviews the architecture
	_, review, err := p.cto.Runtime.Evaluate(ctx, "architecture_review",
		fmt.Sprintf("Review this architecture. Is it sound? Any gaps?\n\n%s", design))
	if err != nil {
		return "", fmt.Errorf("CTO review design: %w", err)
	}

	fmt.Printf("Architecture Review:\n%s\n", review)
	return design, nil
}

// build generates code from the design spec.
func (p *Pipeline) build(ctx context.Context, design string) (string, error) {
	builder, err := p.ensureAgent(ctx, roles.RoleBuilder, "builder")
	if err != nil {
		return "", err
	}

	prompt := fmt.Sprintf("%s\n\nGenerate production-quality code from this specification. Include tests.\n\n%s",
		roles.SystemPrompt(roles.RoleBuilder), design)

	code, err := builder.Runtime.CodeWrite(ctx, prompt, "go")
	if err != nil {
		return "", fmt.Errorf("builder code: %w", err)
	}

	// Write code to workspace
	productDir := p.ws.ProductDir("current")
	err = p.ws.WriteFile(productDir+"/main.go", code)
	if err != nil {
		return "", fmt.Errorf("write code: %w", err)
	}

	fmt.Printf("Code generated: %d bytes\n", len(code))
	return code, nil
}

// review checks code quality and spec compliance.
func (p *Pipeline) review(ctx context.Context, code string, design string) error {
	reviewer, err := p.ensureAgent(ctx, roles.RoleReviewer, "reviewer")
	if err != nil {
		return err
	}

	reviewEvt, review, err := reviewer.Runtime.CodeReview(ctx, code, "go")
	if err != nil {
		return fmt.Errorf("code review: %w", err)
	}

	// Also check spec compliance
	_, specReview, err := reviewer.Runtime.Evaluate(ctx, "spec_compliance",
		fmt.Sprintf("%s\n\nDoes this code match the design spec?\n\nDesign:\n%s\n\nCode:\n%s",
			roles.SystemPrompt(roles.RoleReviewer), design, code))
	if err != nil {
		return fmt.Errorf("spec review: %w", err)
	}

	fmt.Printf("Code Review:\n%s\n\nSpec Compliance:\n%s\n", review, specReview)
	_ = reviewEvt
	return nil
}

// test runs tests and validates behavior.
func (p *Pipeline) test(ctx context.Context, code string) error {
	tester, err := p.ensureAgent(ctx, roles.RoleTester, "tester")
	if err != nil {
		return err
	}

	_, testEval, err := tester.Runtime.Evaluate(ctx, "test_analysis",
		fmt.Sprintf("%s\n\nAnalyze this code. What tests exist? What gaps are there? Write additional integration tests if needed.\n\n%s",
			roles.SystemPrompt(roles.RoleTester), code))
	if err != nil {
		return fmt.Errorf("test analysis: %w", err)
	}

	fmt.Printf("Test Analysis:\n%s\n", testEval)
	return nil
}

// integrate assembles and prepares for deployment.
func (p *Pipeline) integrate(ctx context.Context) error {
	integrator, err := p.ensureAgent(ctx, roles.RoleIntegrator, "integrator")
	if err != nil {
		return err
	}

	_, err = integrator.Runtime.Act(ctx, "integrate", "staging")
	if err != nil {
		return fmt.Errorf("integration: %w", err)
	}

	// Escalate to human for production approval
	humanID := types.MustActorID("actor_human_matt")
	_, err = integrator.Runtime.Escalate(ctx, humanID, "Product ready for human review before production deploy")
	if err != nil {
		return fmt.Errorf("escalate: %w", err)
	}

	fmt.Println("Product assembled and ready for human review.")
	return nil
}

// GuardianWatch runs the Guardian's monitoring loop.
// It checks recent events for policy violations.
func (p *Pipeline) GuardianWatch(ctx context.Context) error {
	events, err := p.guardian.Runtime.Memory(20)
	if err != nil {
		return fmt.Errorf("guardian memory: %w", err)
	}

	if len(events) == 0 {
		return nil
	}

	// Build summary of recent activity for the Guardian to evaluate
	var summary string
	for _, ev := range events {
		summary += fmt.Sprintf("[%s] %s: %s\n", ev.Type().Value(), ev.Source().Value(), ev.ID().Value())
	}

	_, eval, err := p.guardian.Runtime.Evaluate(ctx, "integrity_check",
		fmt.Sprintf("%s\n\nReview these recent events for policy violations, trust anomalies, or authority overreach:\n\n%s",
			roles.SystemPrompt(roles.RoleGuardian), summary))
	if err != nil {
		return fmt.Errorf("guardian evaluate: %w", err)
	}

	fmt.Printf("Guardian Report:\n%s\n", eval)

	// If the Guardian detects issues, emit an alert
	if containsAlert(eval) {
		_, err = p.guardian.Runtime.Emit(event.AgentEscalatedContent{
			AgentID:   p.guardian.Runtime.ID(),
			Authority: types.MustActorID("actor_human_matt"),
			Reason:    eval,
		})
		if err != nil {
			return fmt.Errorf("guardian alert: %w", err)
		}
	}

	return nil
}

// containsAlert checks if the Guardian's evaluation contains an alert keyword.
func containsAlert(eval string) bool {
	upper := strings.ToUpper(eval)
	for _, keyword := range []string{"HALT", "ALERT", "VIOLATION", "QUARANTINE"} {
		if strings.Contains(upper, keyword) {
			return true
		}
	}
	return false
}

// Store returns the shared event graph.
func (p *Pipeline) Store() store.Store {
	return p.store
}

// Agents returns all active agents.
func (p *Pipeline) Agents() map[roles.Role]*roles.Agent {
	return p.agents
}
