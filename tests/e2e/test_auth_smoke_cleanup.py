"""Offline cleanup contracts. No CDP, processes, credentials, or HTTP requests."""
from pathlib import Path
import unittest
import urllib.error
import urllib.parse


class CleanupTests(unittest.TestCase):
    def setUp(self):
        source = Path(__file__).with_name("auth_smoke.py").read_text()
        self.runner = {}
        # Load helper definitions, never the browser execution/finally block.
        exec(compile(source.split("\nexit_code = 0\n")[0], "auth_smoke.py", "exec"), self.runner)
        self.runner["email"] = "owned-test@example.com"
        self.runner["registration_attempted"] = True
        self.runner["owned_messages"] = set()
        self.runner["request"] = self.request
        self.messages = []
        self.deleted = []
        self.read_bodies = []
        self.ignore_pagination = False
        self.delete_works = True
        self.detail_recipient_mismatch = False
        self.bad_absence_status = None
        self.bad_listing = False
        self.arrive_after_delete = False

    def envelope(self, mid, owned=False):
        address = self.runner["email"] if owned else "unrelated@example.com"
        return {"ID": mid, "To": [{"Address": address}]}

    def request(self, url, method="GET", body=None, admin=False):
        parsed = urllib.parse.urlsplit(url)
        self.assertEqual(parsed.scheme + "://" + parsed.netloc, self.runner["INBOX"])
        self.assertFalse(admin)
        if parsed.path == "/api/v1/messages":
            if method == "DELETE":
                self.assertEqual(len(body["IDs"]), 1)
                mid = body["IDs"][0]
                self.assertTrue(any(m["ID"] == mid and self.runner["message_owned"](m) for m in self.messages))
                self.deleted.append(mid)
                if self.delete_works:
                    self.messages = [m for m in self.messages if m["ID"] != mid]
                if self.arrive_after_delete:
                    self.messages.append(self.envelope("late-owned", owned=True))
                    self.arrive_after_delete = False
                return None  # status-only DELETE acknowledgement.
            self.assertEqual(method, "GET")
            if self.bad_listing:
                return {}
            query = urllib.parse.parse_qs(parsed.query)
            start = int(query["start"][0])
            self.assertEqual(query["limit"], ["100"])
            actual_start = 0 if self.ignore_pagination else start
            return {"start": actual_start, "messages": self.messages[actual_start:actual_start + 100]}
        prefix = "/api/v1/message/"
        self.assertTrue(parsed.path.startswith(prefix))
        self.assertEqual(method, "GET")
        mid = urllib.parse.unquote(parsed.path[len(prefix):])
        found = next((m for m in self.messages if m["ID"] == mid), None)
        if found is None:
            code = self.bad_absence_status or 404
            raise urllib.error.HTTPError(url, code, "synthetic status", None, None)
        # No unrelated body may be read, even during cleanup rediscovery.
        self.assertTrue(self.runner["message_owned"](found))
        self.read_bodies.append(mid)
        if self.detail_recipient_mismatch:
            return self.envelope(mid)
        return found

    def test_failed_before_discovery_rediscovers_and_deletes_exact_recipient(self):
        self.messages = [self.envelope("unrelated"), self.envelope("owned", owned=True)]
        self.assertEqual(self.runner["owned_messages"], set())
        self.assertEqual(self.runner["cleanup"](), [])
        self.assertEqual(self.deleted, ["owned"])
        self.assertEqual(self.read_bodies, ["owned"])
        self.assertEqual([m["ID"] for m in self.messages], ["unrelated"])

    def test_owned_message_on_second_page_is_discovered(self):
        self.messages = [self.envelope(str(i)) for i in range(101)] + [self.envelope("owned", owned=True)]
        self.assertEqual(self.runner["cleanup"](), [])
        self.assertEqual(self.deleted, ["owned"])
        self.assertEqual(self.read_bodies, ["owned"])

    def test_capacity_cannot_claim_absence(self):
        self.messages = [self.envelope(str(i)) for i in range(500)]
        self.assertIn("inbox capacity exceeds 500-envelope cleanup bound", self.runner["cleanup"]()[0])
        self.assertEqual(self.deleted, [])

    def test_ignored_pagination_cannot_claim_absence(self):
        self.messages = [self.envelope(str(i)) for i in range(101)]
        self.ignore_pagination = True
        self.assertIn("inbox pagination not honored", self.runner["cleanup"]()[0])

    def test_missing_envelopes_cannot_claim_absence(self):
        self.bad_listing = True
        self.assertIn("inbox envelope listing incomplete", self.runner["cleanup"]()[0])

    def test_changed_recipient_is_not_deleted(self):
        self.messages = [self.envelope("owned", owned=True)]
        self.detail_recipient_mismatch = True
        self.assertIn("cleanup mail ownership mismatch", self.runner["cleanup"]()[0])
        self.assertEqual(self.deleted, [])

    def test_deletion_requires_absence(self):
        self.messages = [self.envelope("owned", owned=True)]
        self.delete_works = False
        self.assertIn("owned mail remains after deletion", self.runner["cleanup"]()[0])

    def test_only_404_establishes_absence(self):
        self.messages = [self.envelope("owned", owned=True)]
        self.bad_absence_status = 403
        self.assertIn("owned mail absence requires HTTP 404", self.runner["cleanup"]()[0])

    def test_new_matching_mail_prevents_cleanup_pass(self):
        self.messages = [self.envelope("owned", owned=True)]
        self.arrive_after_delete = True
        self.assertIn("exact-recipient mail remains after cleanup", self.runner["cleanup"]()[0])


if __name__ == "__main__":
    unittest.main()
