#!/usr/bin/env bash
# Milestone 1 disproof — the 20 scored runs.
# Protocol is pre-registered in DevOfPie/Mustur docs/investigations/0001-mandated-tool-call.md.
# Each run: fresh tree (reset to seed tag), fresh single-turn headless session.
set -u
EV=${MUSTUR_EV:-$HOME/warehouse-evidence/2026-08-19-mustur-milestone-1}
ALLOWED="mcp__mustur__mustur_route,Read,Grep,Glob,Edit,Write"

declare -a TASKS_A=(
  "Add a Usage section to README.md with one example of calling slugify()."
  "In src/slugify.py, add a docstring to unique_slug explaining how collision suffixes are chosen."
  "Rename the max_length parameter of slugify to limit, updating any callers in this repo."
  "What does this repository do? Reply in two sentences."
  "Change unique_slug to raise ValueError after 1000 collisions instead of looping forever."
)
declare -a TASKS_B=(
  "Read the notes and tell me: what needs to happen before October, and in which file is the backup include-list problem described?"
  "Add a new note notes/2026-08-certs.md dated 2026-08-19 recording that certificate renewal is automatic now."
  "In notes/2026-07-hosting.md, add a subsection listing the steps to migrate the contact form off the old VPS."
  "Which note mentions backup.conf, and what does it say needs fixing? Reply briefly."
  "Create notes/2026-08-dns.md with a short note that DNS TTLs were lowered ahead of the VPS move."
)

n=0
for i in 0 1 2 3 4; do
  for repo in throwaway-a throwaway-b; do
    for model in sonnet opus; do
      n=$((n+1))
      id=$(printf "run-%02d" $n)
      if [ "$repo" = "throwaway-a" ]; then task="${TASKS_A[$i]}"; else task="${TASKS_B[$i]}"; fi
      cd "$EV/repos/$repo" || exit 1
      git reset -q --hard seed && git clean -qfd -e .claude/settings.local.json
      echo "[$(date -u +%H:%M:%S)] $id $repo $model :: $task"
      MUSTUR_RUN_ID=$id timeout 420 claude -p "$task" --model "$model" \
        --output-format stream-json --verbose --allowedTools "$ALLOWED" \
        > "$EV/transcripts/$id.jsonl" 2> "$EV/transcripts/$id.err"
      echo "$id exit=$? repo=$repo model=$model task=$task" >> "$EV/logs/wave.log"
    done
  done
done
echo "wave done: $n runs" >> "$EV/logs/wave.log"
