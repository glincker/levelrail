import sqlite3

def find_missing():
    conn = sqlite3.connect('/tmp/test.db')
    cursor = conn.cursor()
    cursor.execute("SELECT name FROM sqlite_master WHERE type='table';")
    tables = [row[0] for row in cursor.fetchall()]

    for table in tables:
        cursor.execute(f"PRAGMA foreign_key_list('{table}');")
        fks = cursor.fetchall()
        # id, seq, table, from, to, on_update, on_delete, match
        if not fks:
            continue

        cursor.execute(f"PRAGMA index_list('{table}');")
        indices = cursor.fetchall()

        index_cols = []
        for idx in indices:
            idx_name = idx[1]
            cursor.execute(f"PRAGMA index_info('{idx_name}');")
            # seqno, cid, name
            cols = [row[2] for row in cursor.fetchall()]
            if cols:
                index_cols.append(cols[0]) # naive, just first col

        for fk in fks:
            from_col = fk[3]
            if from_col not in index_cols:
                print(f"Missing index on {table}({from_col}) -> {fk[2]}({fk[4]})")

find_missing()
