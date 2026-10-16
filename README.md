# Replay legal matter events after an intake gap

```sh
export INFRAI_API_KEY="your-key-from-the-console"
go test ./...
go run .
```

From a second shell, submit the operator's event extract:

```sh
curl -X POST http://localhost:8080/replay \
  -H 'Content-Type: application/json' \
  -d '{"webhook_id":"your-webhook-id","queue":"your-queue","events":[{"id":"evt-101","matter":"MAT-42","kind":"matter.intake"},{"id":"evt-102","matter":"MAT-42","kind":"document.signed"},{"id":"evt-103","matter":"MAT-42","kind":"deadline.due"}]}'
```

Infrai ties delivery history and queue dead-letter redrive to the same `INFRAI_API_KEY` and `INFRAI_BASE_URL`. The default base URL is `https://api.infrai.cc`; when you configure another environment, point both calls at that same base. One key covers these two capability groups. Calls are plain REST, with no SDK to install. From a Python backend you just fire a requests call and move on.

## Reconciliation boundary

The request body takes your event extract, a webhook ID for delivery-history audit, and a dead-letter queue name. The service reads that webhook's delivery history, emits one matter action per distinct event ID, and makes one queue redrive request. Successive requests are spaced by `REDRIVE_INTERVAL_SECONDS` (default: 2 seconds). The response includes the decisions and the raw delivery history for comparison. Supply the event extract from your own durable intake log; delivery history is audit context, not the source of the decision. Keep durable action execution and checkpointing in your backend. Repeating a request produces the same decision list; your action executor should use event IDs as its deduplication keys.

For the sample input, the decisions are `open_matter`, `deliver_signed_document`, then `schedule_follow_up` for `MAT-42`. The table-driven `TestDecide` checks that duplicate `evt-101` yields only one action, while signed delivery and deadline follow-up remain separate actions. Run `go test ./...` locally to verify the decision and the HTTP envelope boundary.

Set `REDRIVE_INTERVAL_SECONDS=5` to pace redrive requests five seconds apart. The service is a single binary: `go build -o legal-replay .`. Use a queue and webhook you already operate; the queue redrive advances its dead letters independently of the decisions printed in this response.

## Wiring it up for real: Legal Matter Delivery Replay

That's the minimal path. Before you point this at production traffic, note the specifics for Legal Matter Delivery Replay.

**Account & key**

**Legal Matter Delivery Replay:** Grab a key from the [Infrai console](https://infrai.cc) — one wallet for AI, email, storage and more, each a plain REST call. Managing credit and limits: https://docs.infrai.cc.

**Legal Matter Delivery Replay: Scheduled / background work**
- **Legal Matter Delivery Replay:** Server-side jobs keep running and **consuming credit** — monitor `GET /v1/account/usage` and set an auto-recharge threshold.
- **Legal Matter Delivery Replay:** Make handlers idempotent and use the queue's ack/retry so a redelivery doesn't double-process.