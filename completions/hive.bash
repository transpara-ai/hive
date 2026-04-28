# Bash completion for the hive CLI
# Source this file: source completions/hive.bash
# Or copy to: /etc/bash_completion.d/hive

_hive_completions() {
    local cur prev words cword
    _init_completion || return

    # Top-level verbs
    local verbs="civilization pipeline role ingest council"

    # Subverbs
    local sub_verbs="run daemon"

    case "${#words[@]}" in
        2)
            # hive <verb>
            COMPREPLY=($(compgen -W "$verbs help" -- "$cur"))
            return
            ;;
        3)
            # hive <verb> <subverb>
            case "${words[1]}" in
                civilization|pipeline)
                    COMPREPLY=($(compgen -W "$sub_verbs" -- "$cur"))
                    return
                    ;;
                role)
                    # role name is freeform, no completion
                    return
                    ;;
                ingest)
                    # expects a file
                    _filedir '*.md'
                    return
                    ;;
            esac
            ;;
        4)
            # hive role <name> <subverb>
            if [[ "${words[1]}" == "role" ]]; then
                COMPREPLY=($(compgen -W "$sub_verbs" -- "$cur"))
                return
            fi
            ;;
    esac

    # Flag completion based on verb
    case "${words[1]}" in
        civilization)
            local civ_flags="--human --idea --spec --seed-spec --store --repo --catalog --approve-requests --approve-roles --space --api"
            if [[ "$cur" == -* ]]; then
                COMPREPLY=($(compgen -W "$civ_flags" -- "$cur"))
                return
            fi
            # File completion for path flags
            case "$prev" in
                --catalog|--spec|--seed-spec)
                    _filedir 'yaml yml'
                    return
                    ;;
                --repo)
                    _filedir -d
                    return
                    ;;
            esac
            ;;
        pipeline)
            local pipe_flags="--space --api --repo --agent-id --store --repos --budget --pr --worktrees --auto-clone --interval"
            if [[ "$cur" == -* ]]; then
                COMPREPLY=($(compgen -W "$pipe_flags" -- "$cur"))
                return
            fi
            case "$prev" in
                --repo)
                    _filedir -d
                    return
                    ;;
            esac
            ;;
        role)
            local role_flags="--space --api --repo --agent-id --budget --pr"
            if [[ "$cur" == -* ]]; then
                COMPREPLY=($(compgen -W "$role_flags" -- "$cur"))
                return
            fi
            case "$prev" in
                --repo)
                    _filedir -d
                    return
                    ;;
            esac
            ;;
        ingest)
            local ingest_flags="--space --api --priority"
            if [[ "$cur" == -* ]]; then
                COMPREPLY=($(compgen -W "$ingest_flags" -- "$cur"))
                return
            fi
            case "$prev" in
                --priority)
                    COMPREPLY=($(compgen -W "high normal low" -- "$cur"))
                    return
                    ;;
            esac
            ;;
        council)
            local council_flags="--space --api --repo --budget --topic"
            if [[ "$cur" == -* ]]; then
                COMPREPLY=($(compgen -W "$council_flags" -- "$cur"))
                return
            fi
            case "$prev" in
                --repo)
                    _filedir -d
                    return
                    ;;
            esac
            ;;
    esac
}

complete -F _hive_completions hive
