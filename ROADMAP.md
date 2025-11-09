# 🗓️ Lumen Technical Roadmap

> This document outlines the current development plan and the progressive decentralization schedule for the Lumen network.  
> Dates are indicative and may evolve based on stability and community feedback.

---

## 🎯 Overall Goal
Stabilize the core chain, validate all security primitives (notably PQC and gasless economics), and progressively open the validator set to trusted community developers.

---

## 🧱 Phase 1 — Code Hardening & Peer Review  
**Period:** Early - Mid November 2025  
**Objective:** Complete all internal reviews and finalize the security layer.

**Focus:**
- Finalize PQC enforcement and anti-spam logic  
- Validate the “gasless + fixed 1% fee” transaction model  
- Improve documentation, CI, and release security guards  
- Run full local e2e and gateway stability tests  

**Deliverables:**
- Stable branch `v0.9.0-rc`  
- Green CI (tests + security checks)  
- Updated developer documentation  

---

## 🧪 Phase 2 — Private Mainnet (Trusted Network)  
**Period:** Mid November → Mid December 2025  
**Objective:** Run the production chain with a small circle of trusted developers before any external validator joins.

**Focus:**
- Deploy the first **mainnet instance** (non-public)  
- Operate with the real consensus, modules, and token logic  
- Conduct performance, spam, and PQC stress testing  
- Monitor block propagation, mempool health, and gateway sync  
- Tune rate-limit and tax parameters for production load  

**Deliverables:**
- Tag `v0.9.0-mainnet-internal`  
- Stability & performance report  
- Ready state for community expansion (Phase 3)  

---

## 🤝 Phase 3 — Early Community Participation  
**Period:** Mid December 2025 → January 2026  
**Objective:** Gradual opening to external contributors and small validator groups.

**Focus:**
- Release setup & validator onboarding documentation  
- Open selected repositories for contribution  
- Onboard first community validators  
- Gather feedback on UX, performance, and governance  

**Deliverables:**
- Public testnet (trusted-validator phase)  
- Community dashboards and metrics  

---

## 🌐 Phase 4 — Progressive Decentralization  
**Period:** February 2026 → onward  
**Objective:** Expand validator participation in a controlled and transparent manner.

**Focus:**
- Add external validators progressively  
- Maintain high-availability RPC and gateway nodes  
- Strengthen monitoring and anti-abuse protection  
- Validate governance and upgrade procedures in real conditions  

**Deliverables:**
- Public validator onboarding  
- Milestone tag `v1.0.0` (Mainnet Public Launch)  

---

## 📘 Notes
- Lumen is a **community-driven chain**, not a fundraising or profit-oriented project.  
- No external audits or funding rounds are planned — all reviews are open and peer-based.  
- A key product built on top of Lumen is under development and will be introduced soon after launch.  
- The `main` branch remains **protected** until the end of Phase 2.  
- All official releases are **GPG-signed** and reproducible.

---

*Last updated: November 2025*
