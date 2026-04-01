# Stockyard Telegraph

**Webhook sender.** The opposite of Corral. Define events, register subscriber URLs, fire events and Telegraph delivers with HMAC signing, retry, and full delivery log. Single binary, no external dependencies.

Part of the [Stockyard](https://stockyard.dev) suite of self-hosted developer tools.

## Quick Start

```bash
curl -sfL https://stockyard.dev/install/telegraph | sh
telegraph
```

Dashboard at [http://localhost:8850/ui](http://localhost:8850/ui)

## Usage

```bash
# Define an event type
curl -X POST http://localhost:8850/api/events \
  -H "Content-Type: application/json" \
  -d '{"name":"order.created","description":"Fired on new orders"}'

# Register a subscriber
curl -X POST http://localhost:8850/api/events/order.created/subscriptions \
  -H "Content-Type: application/json" \
  -d '{"url":"https://partner.com/webhooks/orders"}'

# Fire the event (delivers to all subscribers)
curl -X POST http://localhost:8850/api/events/order.created/fire \
  -H "Content-Type: application/json" \
  -d '{"order_id":456,"total":99.99}'

# Check delivery log
curl http://localhost:8850/api/deliveries
```

Pro subscribers get HMAC-SHA256 signed payloads via `X-Telegraph-Signature` header.

## API

| Method | Path | Description |
|--------|------|-------------|
| POST | /api/events | Define event type |
| GET | /api/events | List event types |
| GET | /api/events/{name} | Event detail |
| DELETE | /api/events/{name} | Remove event |
| POST | /api/events/{name}/subscriptions | Register subscriber |
| GET | /api/events/{name}/subscriptions | List subscribers |
| DELETE | /api/subscriptions/{id} | Remove subscriber |
| POST | /api/events/{name}/fire | Fire event to all subscribers |
| GET | /api/deliveries | All deliveries |
| GET | /api/events/{name}/deliveries | Event deliveries |
| GET | /api/deliveries/{id} | Delivery detail |
| POST | /api/deliveries/{id}/retry | Retry failed delivery |

## Free vs Pro

| Feature | Free | Pro ($2.99/mo) |
|---------|------|----------------|
| Event types | 3 | Unlimited |
| Subscriptions | 5 | Unlimited |
| Fires/month | 1,000 | Unlimited |
| HMAC-SHA256 signing | — | ✓ |
| Auto-retry with backoff | — | ✓ |
| Delivery log | 7 days | 90 days |

## License

Apache 2.0 — see [LICENSE](LICENSE).
