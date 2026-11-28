# FX

The FX module owns the rate, quote, position limit, and execution record. The ledger owns each currency's journal and conversion account balance.

## Rates and quotes

| ID | Requirement | Acceptance check |
|---|---|---|
| FX-01 | Store a rate as a positive mantissa and a decimal scale with observation time, receipt time, source, and pair. Use bounded exact arithmetic. | Reject an invalid input, a stale rate, an unsupported pair, and an overflow. |
| FX-02 | Record a named rule for inverse and cross rates. Hold an outlier before use. Version the markup for the pair and direction. | Inject a rate jump and missing rates. Check that a held or expired rate cannot price a quote. |
| FX-03 | Quote a fixed buy or sell amount. Record both currency amounts, the rate, markup, and expiry. Round once in the target minor unit. Round up the sell amount for a fixed buy. | Compare with an independent exact oracle. Cover exponents, rounding ties, fixed-side division, and large values. |
| FX-04 | Bind a quote to its tenant and input. Use it once. Reject an expired quote, changed input, or excess rate movement under the chosen policy. | Race two executions of one quote. Replay the same request. Verify the stored quote and exact result. |

The design candidate uses a signed 64-bit positive mantissa and a scale up to 18. Name the rounding rules in the contract. Keep rate precision, markup, validity, and movement thresholds visible in the fixture data.

## Execution and recovery

1. Reserve position capacity under a concurrency guard.
2. Record the execution and stable keys before the first post.
3. Post the sold-currency journal. Record its result.
4. Post the bought-currency journal with its own key. Mark completion only after both posts exist.
5. Release or convert the position reservation according to the final result.

**FX-05.** Stop after the first journal. The intermediate position remains visible. Resume with the same keys and post only the missing leg.

**FX-06.** If the second leg cannot post, keep the execution open and alert an operator. Resolve it by completing the leg or by approved corrective journals. Test that no limit breach or duplicate journal appears during concurrent recovery.

**Open design decision.** FX-05 and FX-06 describe recovery with a committed first leg. The [candidate atomic group](../proposals/ledger-cross-border-payments.md#conflict-and-amendment-register) would change that local failure state. Review and explicitly accept any amendment to the FX contract and [Ledger ownership decision](../adr/0002-ledger-and-data-ownership.md) before implementation.

Direct pair support serves the first pilot. Advanced pricing and FX reporting belong to later work. The [decision register](../open-decisions.md#money-and-recovery) covers slow payment methods that outlive a quote.
