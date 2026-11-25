"""Stream change nudges over SSE with a scoped token.

Install the SDK from the checkout first: pip install -e ./sdk/python
The stream ends when the server closes it or on error. Reconnect and refetch
state in that case; this example exits after one stream.
"""
import os

from levelrail import Client

BASE_URL = "https://levelrail.example.com"


def main() -> None:
    client = Client(BASE_URL, os.environ["LEVELRAIL_TOKEN"])
    for event in client.changes():
        if event.get("heartbeat"):
            continue
        print("changed:", event.get("topics"))


if __name__ == "__main__":
    main()
