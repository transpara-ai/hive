#compdef hive

# Zsh completion for the hive CLI
# Add to fpath: fpath=(path/to/hive/completions $fpath)
# Or source directly: source completions/hive.zsh

_hive() {
    local -a verbs sub_verbs

    verbs=(
        'civilization:Multi-agent runtime'
        'pipeline:Scout→Builder→Critic state machine'
        'role:Single agent runner'
        'ingest:Post a markdown spec as a task'
        'council:Convene one deliberation'
    )

    sub_verbs=(
        'run:One-shot execution'
        'daemon:Long-running mode'
    )

    _arguments -C \
        '1:verb:->verb' \
        '*::arg:->args'

    case "$state" in
        verb)
            _describe 'verb' verbs
            ;;
        args)
            case "${words[1]}" in
                civilization)
                    _arguments -C \
                        '1:subverb:->subverb' \
                        '*::flag:->civ_flags'
                    case "$state" in
                        subverb)
                            _describe 'subverb' sub_verbs
                            ;;
                        civ_flags)
                            _arguments \
                                '--human[Operator name]:name:' \
                                '--idea[Freeform seed]:idea:' \
                                '--spec[Markdown spec file]:file:_files -g "*.md"' \
                                '--seed-spec[Initial spec for daemon]:file:_files -g "*.md"' \
                                '--store[Store DSN]:dsn:' \
                                '--repo[Path to repo for Operate]:dir:_files -/' \
                                '--catalog[Custom YAML model catalog]:file:_files -g "*.{yaml,yml}"' \
                                '--approve-requests[Auto-approve authority requests]' \
                                '--approve-roles[Auto-approve role proposals]' \
                                '--space[lovyou.ai space slug]:slug:' \
                                '--api[lovyou.ai API base URL]:url:'
                            ;;
                    esac
                    ;;
                pipeline)
                    _arguments -C \
                        '1:subverb:->subverb' \
                        '*::flag:->pipe_flags'
                    case "$state" in
                        subverb)
                            _describe 'subverb' sub_verbs
                            ;;
                        pipe_flags)
                            _arguments \
                                '--space[lovyou.ai space slug]:slug:' \
                                '--api[lovyou.ai API base URL]:url:' \
                                '--repo[Path to repo]:dir:_files -/' \
                                '--agent-id[Agent user ID]:id:' \
                                '--store[Store DSN]:dsn:' \
                                '--repos[Named repos name=path]:repos:' \
                                '--budget[Daily budget in USD]:budget:' \
                                '--pr[Create feature branch and open PR]' \
                                '--worktrees[Each Builder task gets its own git worktree]' \
                                '--auto-clone[Clone missing repos from registry]' \
                                '--interval[Pipeline cycle interval]:duration:'
                            ;;
                    esac
                    ;;
                role)
                    _arguments -C \
                        '1:role-name:' \
                        '2:subverb:->subverb' \
                        '*::flag:->role_flags'
                    case "$state" in
                        subverb)
                            _describe 'subverb' sub_verbs
                            ;;
                        role_flags)
                            _arguments \
                                '--space[lovyou.ai space slug]:slug:' \
                                '--api[lovyou.ai API base URL]:url:' \
                                '--repo[Path to repo]:dir:_files -/' \
                                '--agent-id[Agent user ID]:id:' \
                                '--budget[Daily budget in USD]:budget:' \
                                '--pr[Create feature branch and open PR]'
                            ;;
                    esac
                    ;;
                ingest)
                    _arguments \
                        '1:spec-file:_files -g "*.md"' \
                        '--space[lovyou.ai space slug]:slug:' \
                        '--api[lovyou.ai API base URL]:url:' \
                        '--priority[Task priority]:priority:(high normal low)'
                    ;;
                council)
                    _arguments \
                        '--space[lovyou.ai space slug]:slug:' \
                        '--api[lovyou.ai API base URL]:url:' \
                        '--repo[Path to repo]:dir:_files -/' \
                        '--budget[Daily budget in USD]:budget:' \
                        '--topic[Focus the council on a specific question]:topic:'
                    ;;
            esac
            ;;
    esac
}

_hive "$@"
