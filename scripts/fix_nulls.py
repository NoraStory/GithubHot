import sqlite3

c = sqlite3.connect("/opt/githubhot/data/githubhot.db")
c.execute("UPDATE sources SET last_fetched_at='' WHERE last_fetched_at IS NULL")
c.commit()
print("修正行数:", c.total_changes)
print("仍为空:", c.execute("SELECT COUNT(*) FROM sources WHERE last_fetched_at IS NULL").fetchone()[0])
