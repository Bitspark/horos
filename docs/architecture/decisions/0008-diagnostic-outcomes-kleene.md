# Diagnostic outcomes preserve incomplete checks

The boolean recognizer predates the diagnostic API and keeps its bounded
behavior. A caller needing completion evidence uses `Diagnose`/`diagnose`, not an
interpretation of false as a completed mismatch.

The four verdicts are conformance, mismatch, malformed descriptor and exhaustion.
The profile identifies the applicable limits. `not(exhausted)` is exhausted.
For a union, a conforming branch decides conformance even if another branch is
exhausted. Otherwise exhaustion takes precedence over mismatch. For intersection,
a mismatching branch decides mismatch even if another branch is exhausted;
otherwise exhaustion takes precedence over conformance. This is independent of
branch traversal order. A malformed descriptor is rejected before value checking.

The implementations and their boundary tests retain these rules in 0.6.0.
Completed diagnostic outcomes agree with the boolean recognizer. An exhausted
outcome does not establish either membership or non-membership.
