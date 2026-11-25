"""Walk every app across pages with a scoped token.

Install the SDK from the checkout first: pip install -e ./sdk/python
"""
import os

from levelrail import Client

BASE_URL = "https://levelrail.example.com"


def main() -> None:
    client = Client(BASE_URL, os.environ["LEVELRAIL_TOKEN"])
    for app in client.all_apps(limit=100):
        print(app.get("name"))


if __name__ == "__main__":
    main()
