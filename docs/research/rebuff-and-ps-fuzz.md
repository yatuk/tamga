# Rebuff and ps-fuzz: take or not

Read on 2026-10-12 for sprint item S6-5. This page records what the two
projects are, under which licence, how alive they are, and whether Tamga
should use them. Nothing was installed or run and no file from either
project is in this repository. The LICENSE files, `pyproject.toml` files, a
Rebuff source file and the ps-fuzz README were read as raw text from
`raw.githubusercontent.com`; PyPI metadata was read from the PyPI JSON API.
The GitHub repository and commit pages were read through a fetch tool that
returns a summary of the page; what rests on that is said where it is used,
and the [not verified](#not-verified) list says what was not checked.

## Summary

| | Rebuff | ps-fuzz (Prompt Fuzzer) |
|---|---|---|
| Decision | **Don't take** | **Don't take into the repository.** May be run by hand against a test deployment; one idea taken |
| Licence (LICENSE file) | `Apache-2.0` | `MIT` |
| Copyright | "The Rebuff AI Authors" (2023) | "PROMPT SECURITY LTD" (2024) |
| State | Archived on 2025-05-16; last commit 2024-01-25 | Active; release 2.1.1 on 2026-08-18 |
| Needs | OpenAI key, Pinecone (or Chroma), Supabase for self-hosting | An API key for an attacker model and one for the target model (or Ollama / an OpenAI-compatible endpoint) |

- **Rebuff**: archived, its useful parts are the ones Tamga already has
  (phrase heuristics, canary tokens), and the rest sends every prompt to
  OpenAI and a hosted vector database, which a self-hosted proxy that keeps
  prompts in the building cannot do by default.
- **ps-fuzz**: it tests how well a *system prompt and model* hold up, with
  an LLM writing the attacks and an LLM judging them. That is not what
  Tamga's red-team gate measures (whether the proxy's scanners catch a
  fixed set of prompts, deterministically and offline), it costs API calls,
  and its results change from run to run. Useful as a manual tool, not as a
  dependency or a CI step.

## Rebuff (protectai/rebuff)

**Licence.** The LICENSE file is the Apache License 2.0 text, SPDX
`Apache-2.0`, with the appendix filled in as "Copyright 2023 The Rebuff AI
Authors" (line 189):
<https://raw.githubusercontent.com/protectai/rebuff/main/LICENSE>. There
is no `NOTICE` file at the root (HTTP 404). PyPI agrees: package `rebuff`
0.1.1, `license: Apache-2.0`, classifier "OSI Approved :: Apache Software
License", author "ProtectAI" (<https://pypi.org/pypi/rebuff/json>).
`python-sdk/pyproject.toml` says `license = "Apache License 2.0"`.

**Maintenance.** The repository page says "This repository was archived by
the owner on May 16, 2025. It is now read-only." It shows 345 commits, 26
open issues, 6 open pull requests and no GitHub release (fetch summary of
<https://github.com/protectai/rebuff>). The newest commit on `main` is from
2024-01-25 ("Allow trusted people to run GitHub action tests (#94)", fetch
summary of <https://github.com/protectai/rebuff/commits/main>). The last
PyPI upload is 0.1.1 on 2024-01-20. It requires Python `>=3.8.1,<3.13`, so
it does not install on 3.13, and pins `langchain <0.2`, `openai <2`,
`pinecone-client <4`, `langchain-openai <0.0.4`. The README itself says
"Rebuff is still a prototype and cannot provide 100% protection against
prompt injection attacks!"

**What it does.** Four layers, per the README: heuristics on the input, an
LLM asked whether the input is an attack, a vector database of embeddings
of earlier attacks, and canary tokens added to the prompt and looked for in
the answer. The heuristic layer
(`python-sdk/rebuff/detect_pi_heuristics.py`) builds every sentence from
lists of verbs ("Ignore", "Disregard", "Bypass", ...), adjectives
("previous", "above", ...) and objects ("instructions", "context", ...)
and scores the input by `difflib.SequenceMatcher` similarity to them.
English only. The LLM and vector layers need an OpenAI key and Pinecone
(the README also names Chroma); self-hosting the service needs "Supabase,
OpenAI, and a vector database".

**Against what Tamga has.**

- The heuristic layer is a subset of `proxy/internal/scanner/injection.go`,
  which already scores "ignore previous", "disregard previous" and their
  relatives, and goes further: leetspeak, spelled-out and paraphrased
  forms, role injection, other languages (`injection_i18n.go`) and Turkish
  with its casing and suffixes (`injection_turkish.go`).
- Canary tokens are in Tamga (`canary:` in the policy,
  `docs/operations.md#canary-tokens-system-prompt-leak-detection`), done
  in the proxy, with the token kept out of events.
- The LLM check overlaps with the optional inline classifier
  (`classifier/`), which is a local model and sends nothing out. Rebuff's
  sends the prompt to OpenAI. `docs/threat-model.md` treats request text as
  an asset that should not leave the deployment; an analyzer that ships it
  to a third party by default works against that.
- The vector store of past attacks is the one idea Tamga does not have. It
  needs an embedding model and a store that learns from traffic, which is
  a design decision of its own (what is stored, for how long, who can
  poison it) and not something to adopt from an archived prototype.

**Licence fit.** Apache-2.0 code can be combined into an AGPL-3.0 work
(the FSF lists Apache-2.0 as compatible with GPLv3, and AGPL-3.0 section 13
allows combining with GPLv3 works); vendoring would mean keeping the
copyright line and licence text and adding an entry to `NOTICE`. As a
dependency it would be one more Python package for the licence gate
(`.github/licence-policy.yml`), where `Apache-2.0` is already common. The
licence is not the obstacle; the state of the project and its data flow are.

**Decision: don't take**, in any form. Nothing in it is missing from Tamga
except the vector store, and that is not worth taking from a read-only
repository with pins that hold back `openai` and `langchain`.

## ps-fuzz (prompt-security/ps-fuzz)

**Licence.** The LICENSE file is the MIT licence text, SPDX `MIT`, with
"Copyright 2024 PROMPT SECURITY LTD":
<https://raw.githubusercontent.com/prompt-security/ps-fuzz/main/LICENSE>.
PyPI agrees: package `prompt-security-fuzzer` 2.1.1, `license: MIT`,
classifier "OSI Approved :: MIT License", author "Prompt Security"
(<https://pypi.org/pypi/prompt-security-fuzzer/json>); `pyproject.toml`
says `license = {text = "MIT"}`.

**Maintenance.** Not archived. 168 commits, 17 open issues, 3 open pull
requests (fetch summary of <https://github.com/prompt-security/ps-fuzz>).
The commit page shows work in bursts: October 2024, June 2025, February
2026, then 2026-08-18 ("Prepare release 2.1.1", Dependabot fixes) and
2026-08-19 (fetch summary of
<https://github.com/prompt-security/ps-fuzz/commits/main>). PyPI uploads:
2.0.0 on 2024-08-01, 2.1.1 on 2026-08-18. It requires Python `>=3.10`
(classifiers list 3.10 and 3.11) and pulls in `openai`, `langchain`,
`langchain-community`, `langchain-ollama`, `chromadb`, `pandas` and
`fastparquet`, several of them pinned to exact versions.

**What it does.** You give it a system prompt. An attacker model writes
attack prompts in 16 attack types (jailbreaks, prompt injection, system
prompt extraction, RAG poisoning, a "Linguistic Evasion" attack that
switches to a non-English language), sends them to a target model that
runs with that system prompt, and the results are counted as "Broken",
"Resilient" or "Errors" per attack type. Attacker and target are chosen by
`--attack-provider`/`--attack-model` and `--target-provider`/`--target-model`;
keys come from environment variables (`OPENAI_API_KEY` by default). A
target can be an Ollama server or any OpenAI-compatible endpoint
(`--openai-base-url`), which means it could be pointed at a Tamga proxy.
There is a batch mode (`-b`), custom benchmark CSVs and an interactive
playground. All of this is from the README
(<https://raw.githubusercontent.com/prompt-security/ps-fuzz/main/README.md>).

**Against what Tamga has.** Tamga's red-team runner (`proxy/cmd/redteam`,
`make redteam`) runs fixed CSVs (`proxy/testdata/redteam/prompts.csv`,
`holdout.csv`, and the Turkish `tr_tuning.csv` and `tr_locked.csv`)
through the scanners and fails CI when precision drops; the stress suite
in `tests/stress/` gates on the bypass count. Both are offline,
deterministic and free. ps-fuzz answers a different question (would the
model obey this attack), needs two paid or self-run models for every run,
and gives a different result each time. Pointed through Tamga it would
mostly measure the upstream model; how it scores a request the proxy
refuses (likely as an error) was not checked.

**Licence fit.** MIT is compatible with AGPL-3.0 in either direction that
matters here. Run as an external tool it imposes nothing on Tamga. Copying
its code or its attack data into the repository would need the copyright
line and licence text kept and a `NOTICE` entry; the bundled attack files
(`ps_fuzz/attack_data/harmful_behavior.csv` and others) carry no source or
licence note of their own on the directory page, so where they came from
would have to be found out first, as was done for the injection phrases in
`NOTICE`.

**Decision: don't take into the repository** (no dependency, no vendored
code or data, no CI job). A developer may run it by hand, in batch mode,
against a test deployment with a local Ollama model as attacker and
target, to look for attack shapes the scanners miss; anything it finds
goes into the red-team CSVs as rows written for Tamga, the way the Turkish
sets were. **Idea taken**: its category list (system prompt extraction,
RAG poisoning, typoglycemia, language switching) is a checklist for
`prompts.csv`, which has 15 `injection` rows and 3 `indirect` rows today.

## Not verified

- **Last-commit dates and issue counts** come from the fetch tool's summary
  of the GitHub pages; the GitHub API was not reachable from this session.
  The open-issue *trend* was not measured, only today's counts.
- **That nothing in Rebuff's history was relicensed**, and that its
  JavaScript SDK carries the same licence as the root file. Only the root
  LICENSE and `python-sdk/pyproject.toml` were read.
- **Where ps-fuzz's attack data comes from** and under which licence.
  `harmful_behavior.csv` is likely AdvBench by its name; that is a guess.
- **Whether ps-fuzz sends anything besides the attack and target model
  calls** (telemetry, update checks). Its source was not read.
- **Whether ps-fuzz 2.1.1 runs on Python 3.12 or 3.13.** The classifiers
  stop at 3.11; `requires-python` does not cap it.
- The FSF compatibility statements above are stated from general knowledge
  and were not re-read on this date. Nothing here is legal advice.
