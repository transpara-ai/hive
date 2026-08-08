# Factory v1 local supervisor

These scripts operate only the fresh, loopback Civilization/Dark Factory v1
stack. They do not read `loop/config.env`, adopt existing listeners, deploy, or
touch retained Operation #86 state.

The supervisor defaults to
`/home/transpara/transpara-ai/runtime/civilization-dark-factory-v1` and the
bundled PostgreSQL 16.14 distribution. Override the four application commands
and their working roots when the corresponding binaries are built:

```bash
export FACTORY_V1_HIVE_DAEMON_CMD='./bin/hive factory-v1 daemon'
export FACTORY_V1_HIVE_OPS_CMD='./bin/hive-ops-api'
export FACTORY_V1_WORK_CMD='./bin/work-server'
export FACTORY_V1_SITE_CMD='./bin/site'

scripts/factory-v1/supervisor.sh init
FACTORY_V1_OPERATION86_PATHS_FILE=/absolute/path/to/protected-paths.txt \
  scripts/factory-v1/supervisor.sh preflight
scripts/factory-v1/supervisor.sh start
scripts/factory-v1/supervisor.sh run
scripts/factory-v1/supervisor.sh status
scripts/factory-v1/supervisor.sh restart
scripts/factory-v1/supervisor.sh stop
```

Each non-comment line in the protected-paths file is an exact file or directory
to preserve. Capture the before/after receipts around the acceptance run:

```bash
scripts/factory-v1/operation86-evidence.sh capture \
  --paths /absolute/path/to/protected-paths.txt \
  --output /absolute/fresh-runtime/receipts/operation86-baseline.json

scripts/factory-v1/operation86-evidence.sh compare \
  --paths /absolute/path/to/protected-paths.txt \
  --baseline /absolute/fresh-runtime/receipts/operation86-baseline.json \
  --output /absolute/fresh-runtime/receipts/operation86-final.json
```

Configuration, PID ownership, logs, manifests, hashes, and JSON receipts are
created under the fresh runtime root with private permissions. `stop` verifies
recorded PID start times and executable hashes before signalling a live PID; it
refuses a reused or otherwise mismatched PID.

`run` is the persistent service-manager entry point. It keeps the supervisor in
the foreground, verifies exact process/listener ownership every ten seconds,
and stops the complete stack on TERM. A component failure exits non-zero so a
service manager configured with `Restart=on-failure` restarts the whole stack;
the example unit disables the start-rate limit so retries continue indefinitely.
The Hive daemon runtime snapshot is bound and preflighted on loopback port
`8084`; fresh and pre-runtime-observation configurations receive the complete
four-key observation wiring, while partial configurations fail closed.

For a user-level systemd deployment, install and adapt
`civilization-factory-v1.service.example`, then run `systemctl --user enable
--now civilization-factory-v1.service`. User lingering must be enabled if the
stack must start without an interactive login. The example's optional
`supervisor-launch.env` may supply local command/root overrides; the unit also
starts when that file is absent.
