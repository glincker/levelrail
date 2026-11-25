"""List one page of apps with a scoped token.

Install the SDK from the checkout first: pip install -e ./sdk/python
"""
import os

from levelrail import Client

BASE_URL = "https://levelrail.example.com"


def main() -> None:
    client = Client(BASE_URL, os.environ["LEVELRAIL_TOKEN"])
    apps, next_cursor = client.list_apps(limit=50)
    for app in apps:
        print(app.get("name"))
    if next_cursor:
        print("next cursor:", next_cursor)


if __name__ == "__main__":
    main()
