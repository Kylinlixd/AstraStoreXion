# Xion NAS Failover Test Implementation Plan

> For agentic workers: use executing-plans to implement this plan task-by-task. Steps use checkbox syntax for tracking.

Goal: Deploy a disposable Xion standby on the Flywheel NAS through the existing FRP 27823 mapping and execute a reversible planned-failover test from the public server to NAS and back.

Architecture: The public server remains the primary Xion at 127.0.0.1:8081. The NAS runs a second copy at 127.0.0.1:18081; FRP exposes only that service to the server as 127.0.0.1:27823. A one-time snapshot copies primary objects before manifests over the existing SSH administration tunnel, then the blog endpoint is switched temporarily during the test. The test ends by restoring the original environment and services.

Tech Stack: Go 1.23 static binary, systemd, FRP 0.70 TCP proxy, SSH/rsync over the existing 27822 mapping, Django/Gunicorn, curl, SHA-256.

---

### Task 1: Build and preflight the standby artifact

Files:
- Create: /tmp/astrastore-xion-nas-test (temporary Linux binary, removed after the test)
- Read: /Users/leexd/AstraStoreXion/deploy/systemd/astrastore-xion.service
- Read: /Users/leexd/AstraStoreXion/docs/superpowers/specs/2026-08-11-nas-failover-test-design.md

- [ ] Step 1: Verify the current checkout is main and tests are clean enough to build

Run:

~~~bash
git status --short --branch
git log -1 --oneline
go test ./...
~~~

Expected: branch is main, only known untracked local artifacts remain, and Go tests exit 0.

- [ ] Step 2: Build a static NAS binary

Run:

~~~bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o /tmp/astrastore-xion-nas ./core/apigateway
file /tmp/astrastore-xion-nas
sha256sum /tmp/astrastore-xion-nas
~~~

Expected: an x86-64 ELF executable is created and its checksum is recorded locally without printing service credentials.

- [ ] Step 3: Verify the live primary before changing anything

Run on 192.3.221.53 as root:

~~~bash
systemctl is-active astrastore-xion blog-li frps
curl --fail --silent --show-error http://127.0.0.1:8081/readyz
find /var/lib/astrastore-xion/objects -maxdepth 1 -type f | wc -l
find /var/lib/astrastore-xion/metadata -maxdepth 1 -type f -name '*.json' | wc -l
~~~

Expected: all three services are active, readiness returns 200, and object/manifest counts match.

### Task 2: Seed the NAS standby data and service

Files:
- Create on NAS: /vol1/astrastore-xion-failover-test/astrastore-xion
- Create on NAS: /vol1/astrastore-xion-failover-test/data/objects
- Create on NAS: /vol1/astrastore-xion-failover-test/data/metadata
- Create on NAS: /vol1/astrastore-xion-failover-test/xion.env
- Create on NAS: /vol1/astrastore-xion-failover-test/xion-test.service

- [ ] Step 1: Create an isolated NAS data directory and upload the binary

Run through the existing NAS SSH mapping (27822), using lixd and never embedding its password in a command:

~~~bash
ssh -p 27822 lixd@192.3.221.53 'install -d -m 700 /vol1/astrastore-xion-failover-test/data/objects /vol1/astrastore-xion-failover-test/data/metadata'
scp -P 27822 /tmp/astrastore-xion-nas lixd@192.3.221.53:/vol1/astrastore-xion-failover-test/astrastore-xion
ssh -p 27822 lixd@192.3.221.53 'chmod 700 /vol1/astrastore-xion-failover-test/astrastore-xion'
~~~

Expected: the binary and empty data directories exist on /vol1; no existing NAS application path is touched.

- [ ] Step 2: Copy the primary environment without exposing the token

Create /vol1/astrastore-xion-failover-test/xion.env with mode 600, transferring the primary token through stdin only:

The resulting file contains the three fixed settings above plus the XION_SERVICE_TOKEN line copied from the protected primary environment.

The actual transfer command is:

~~~bash
{
  sed -n 's/^XION_SERVICE_TOKEN=.*/&/p' /etc/astrastore-xion.env
  printf '%s\n' 'XION_LISTEN_ADDR=127.0.0.1:18081' 'XION_DATA_DIR=/vol1/astrastore-xion-failover-test/data' 'XION_MAX_UPLOAD_BYTES=52428800'
} | ssh -p 27822 lixd@127.0.0.1 'umask 077; install -d -m 700 /vol1/astrastore-xion-failover-test; cat > /vol1/astrastore-xion-failover-test/xion.env; chmod 600 /vol1/astrastore-xion-failover-test/xion.env'
~~~

Expected: the file is readable only by the test service account and the token never appears in terminal output, Git, or this plan.

- [ ] Step 3: Copy objects first and manifests second

Run from the public server as root, targeting only the isolated NAS directory:

~~~bash
rsync -a --delete -e 'ssh -p 27822' /var/lib/astrastore-xion/objects/ lixd@127.0.0.1:/vol1/astrastore-xion-failover-test/data/objects/
rsync -a --delete -e 'ssh -p 27822' /var/lib/astrastore-xion/metadata/ lixd@127.0.0.1:/vol1/astrastore-xion-failover-test/data/metadata/
~~~

Expected: every primary object and JSON manifest exists on NAS; counts match before the standby starts.

- [ ] Step 4: Create and start a disposable NAS systemd unit

Install this unit on NAS, then start it:

~~~ini
[Unit]
Description=AstraStoreXion NAS failover test instance
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=lixd
Group=Users
EnvironmentFile=/vol1/astrastore-xion-failover-test/xion.env
ExecStart=/vol1/astrastore-xion-failover-test/astrastore-xion
Restart=on-failure
RestartSec=2s
UMask=0077
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/vol1/astrastore-xion-failover-test

[Install]
WantedBy=multi-user.target
~~~

Run:

~~~bash
sudo systemctl daemon-reload
sudo systemctl enable --now xion-failover-test.service
~~~

Expected: NAS 127.0.0.1:18081/readyz and server 127.0.0.1:27823/readyz both return 200.

### Task 3: Execute the planned failover

Files:
- Backup on server: the timestamped directory created in Task 3 Step 1
- Modify temporarily: /opt/blog_li/.env (XION_BASE_URL only)

- [ ] Step 1: Record rollback state

Run on the public server:

~~~bash
stamp=$(date +%Y%m%d-%H%M%S)
install -d -m 700 "/root/rollback/xion-failover-$stamp"
cp -a /opt/blog_li/.env "/root/rollback/xion-failover-$stamp/blog.env"
systemctl is-active astrastore-xion blog-li > "/root/rollback/xion-failover-$stamp/services.before"
~~~

Expected: the original blog environment and service states are recoverable.

- [ ] Step 2: Stop only the primary Xion service

Run:

~~~bash
systemctl stop astrastore-xion
! curl --silent --show-error --fail http://127.0.0.1:8081/readyz
curl --fail --silent --show-error http://127.0.0.1:27823/readyz
~~~

Expected: primary readiness fails and NAS readiness succeeds.

- [ ] Step 3: Switch Django to the FRP-backed standby

Change only XION_BASE_URL in /opt/blog_li/.env:

~~~text
XION_BASE_URL=http://127.0.0.1:27823
~~~

Then run:

~~~bash
systemctl restart blog-li
systemctl is-active blog-li
~~~

Expected: Gunicorn is active and uses NAS without changing the public Nginx URL.

- [ ] Step 4: Verify read and write paths on NAS

Use an existing Xion file ID from preflight and a newly generated temporary fixture. Verify the existing download checksum, then upload/download/delete the temporary fixture through 127.0.0.1:27823 using the service token. Do not retain the temporary object.

Expected: existing data remains readable and the temporary upload lifecycle succeeds through NAS.

### Task 4: Restore the primary and verify rollback

Files:
- Restore: /opt/blog_li/.env
- Remove after verification: /vol1/astrastore-xion-failover-test

- [ ] Step 1: Stop the disposable NAS service

Run on NAS:

~~~bash
sudo systemctl disable --now xion-failover-test.service
~~~

Expected: 27823/readyz fails after the standby stops.

- [ ] Step 2: Restore the primary blog environment and start Xion

Run on the public server using the timestamped backup:

~~~bash
backup_dir=$(find /root/rollback -maxdepth 1 -type d -name 'xion-failover-*' -printf '%T@ %p\n' | sort -nr | head -1 | cut -d' ' -f2-)
test -n "$backup_dir"
cp -a "$backup_dir/blog.env" /opt/blog_li/.env
systemctl start astrastore-xion
systemctl restart blog-li
systemctl is-active astrastore-xion blog-li
curl --fail --silent --show-error http://127.0.0.1:8081/readyz
~~~

Expected: both services are active and primary readiness returns 200.

- [ ] Step 3: Verify public response and clean only the disposable NAS path

Run:

~~~bash
curl --fail --silent --show-error https://leexd.top/blog >/dev/null
sudo rm -rf -- /vol1/astrastore-xion-failover-test
~~~

Expected: public blog responds successfully; only the explicitly named disposable NAS directory is removed.

- [ ] Step 4: Record results and limitations

Record readiness responses, object/manifest counts, checksum comparison, cutover duration, restore duration, and explicitly state that a full server outage also removes the frps endpoint and is not proven by this test.

### Task 5: Verify repository state

Files:
- Modify: none beyond the approved design and plan documents

- [ ] Step 1: Run repository verification

Run:

~~~bash
git status --short --branch
git log -2 --oneline
go test ./...
~~~

Expected: current branch is main, no tracked deployment artifacts are left in the worktree, and Go tests pass.
