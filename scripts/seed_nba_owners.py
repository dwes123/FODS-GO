#!/usr/bin/env python3
"""
Seeds NBA team owners on staging.

Reads a hardcoded list of (team_or_agency_name, owner_email) pairs from The
Dynasty Association ownership roster. For each email that doesn't yet have a
user account in fantasy_db_staging, generates a placeholder user row with an
unusable bcrypt password hash. Then wires up team_owners (basketball DB) for
real teams and agency_members (basketball DB) for the two agencies.

Outputs two SQL files:
  /tmp/seed_users.sql       — applied to fantasy_db_staging
  /tmp/seed_ownership.sql   — applied to fantasy_basketball_db_staging

Both transactions use ON CONFLICT DO NOTHING so the script is idempotent.
"""

import re
import sys
import uuid

# --- Inputs --- ------------------------------------------------------------

OWNERSHIP = [
    # (team-or-agency name, owner email, kind)
    ("Billy the Kid Sports Agency", "billys150@yahoo.com",         "agency"),
    ("Memphis Grizzlies",           "colinhoch7@gmail.com",        "team"),
    ("The Boys Sports Agency",      "bjhenderson2@gmail.com",      "agency"),
    ("Atlanta Hawks",               "to64time@gmail.com",          "team"),
    ("Boston Celtics",              "michael.connor.walsh@gmail.com", "team"),
    ("Boston Celtics",              "wilsonmikey794@gmail.com",    "team"),
    ("Brooklyn Nets",               "djwes487@gmail.com",          "team"),
    ("Charlotte Hornets",           "vangundykeith2@icloud.com",   "team"),
    ("Chicago Bulls",               "cunca74@yahoo.de",            "team"),
    ("Cleveland Cavaliers",         "coachkb1042@gmail.com",       "team"),
    ("Dallas Mavericks",            "nstithem19@gmail.com",        "team"),
    ("Denver Nuggets",              "friedemannsamuel1@gmail.com", "team"),
    ("Detroit Pistons",             "bryancarley6@gmail.com",      "team"),
    ("Golden State Warriors",       "jasonkaoki1@gmail.com",       "team"),
    ("Houston Rockets",             "mauricealvarado17@gmail.com", "team"),
    ("Indiana Pacers",              "ghijada@hotmail.com",         "team"),
    ("LA Clippers",                 "dmsanderson@hotmail.com",     "team"),
    ("Los Angeles Lakers",          "jstoudt21@gmail.com",         "team"),
    ("Los Angeles Lakers",          "jackson19_2000@yahoo.com",    "team"),
    ("Miami Heat",                  "autcollins13@yahoo.com",      "team"),
    ("Milwaukee Bucks",             "scolheep@gmail.com",          "team"),
    ("Minnesota Timberwolves",      "gkrawczyk2@gmail.com",        "team"),
    ("New Orleans Pelicans",        "mspencer1007@live.com",       "team"),
    ("New York Knicks",             "dayton.schreurs@gmail.com",   "team"),
    ("Oklahoma City Thunder",       "nye_jason@hotmail.com",       "team"),
    ("Orlando Magic",               "zcorby1842@gmail.com",        "team"),
    ("Philadelphia 76ers",          "rrdraper98@gmail.com",        "team"),
    ("Phoenix Suns",                "albertacevedo3@gmail.com",    "team"),
    ("Portland Trail Blazers",      "logan.cunningham4@outlook.com", "team"),
    ("Sacramento Kings",            "chcloutier96@gmail.com",      "team"),
    ("San Antonio Spurs",           "nicktabak7@gmail.com",        "team"),
    ("Toronto Raptors",             "ckeller141@gmail.com",        "team"),
    ("Utah Jazz",                   "sjvcosentino@yahoo.com",      "team"),
    ("Utah Jazz",                   "ryan.mark.bradley@gmail.com", "team"),
    ("Washington Wizards",          "jcable12345@gmail.com",       "team"),
]

# Existing user IDs on staging (from earlier query)
EXISTING_USER_IDS = {
    "coachkb1042@gmail.com":  "89875d59-066a-46c2-a1cc-1a1e9c2d542c",
    "colinhoch7@gmail.com":   "ea421edd-0664-49ae-8f2d-0e52975f1791",
    "djwes487@gmail.com":     "328873e2-a643-4422-a770-1bc2e68b96db",
    "gkrawczyk2@gmail.com":   "b22df72a-8f0c-49c3-8e52-fb4b19b5fe6a",
    "jasonkaoki1@gmail.com":  "1f4282b7-5cb5-4156-934a-8b311cf12c55",
    "jstoudt21@gmail.com":    "0989e997-9cd5-4a3b-b15b-726bc040cae1",
    "nicktabak7@gmail.com":   "38dc5332-7999-475b-9719-cbe5e6d809b7",
}

# Unusable bcrypt hash — valid format, plaintext is a random urlsafe string
# generated and discarded. Login fails with "invalid credentials" which is what
# we want until proper password resets are issued in production.
PLACEHOLDER_HASH = "$2b$10$N5PWKub38l8nu0OVvGJKDe1fcfTSDSeCGPN0iRkalaqK5qIjHx00a"

# Team UUIDs from migrations_nba/003_cba_alignment.sql
TEAM_NAME_TO_ID = {
    "Boston Celtics":         "11111111-1111-1111-1111-aaaaaaaa0001",
    "Brooklyn Nets":          "11111111-1111-1111-1111-aaaaaaaa0002",
    "New York Knicks":        "11111111-1111-1111-1111-aaaaaaaa0003",
    "Philadelphia 76ers":     "11111111-1111-1111-1111-aaaaaaaa0004",
    "Toronto Raptors":        "11111111-1111-1111-1111-aaaaaaaa0005",
    "Chicago Bulls":          "11111111-1111-1111-1111-aaaaaaaa0006",
    "Cleveland Cavaliers":    "11111111-1111-1111-1111-aaaaaaaa0007",
    "Detroit Pistons":        "11111111-1111-1111-1111-aaaaaaaa0008",
    "Indiana Pacers":         "11111111-1111-1111-1111-aaaaaaaa0009",
    "Milwaukee Bucks":        "11111111-1111-1111-1111-aaaaaaaa0010",
    "Atlanta Hawks":          "11111111-1111-1111-1111-aaaaaaaa0011",
    "Charlotte Hornets":      "11111111-1111-1111-1111-aaaaaaaa0012",
    "Miami Heat":             "11111111-1111-1111-1111-aaaaaaaa0013",
    "Orlando Magic":          "11111111-1111-1111-1111-aaaaaaaa0014",
    "Washington Wizards":     "11111111-1111-1111-1111-aaaaaaaa0015",
    "Denver Nuggets":         "11111111-1111-1111-1111-aaaaaaaa0016",
    "Minnesota Timberwolves": "11111111-1111-1111-1111-aaaaaaaa0017",
    "Oklahoma City Thunder":  "11111111-1111-1111-1111-aaaaaaaa0018",
    "Portland Trail Blazers": "11111111-1111-1111-1111-aaaaaaaa0019",
    "Utah Jazz":              "11111111-1111-1111-1111-aaaaaaaa0020",
    "Golden State Warriors":  "11111111-1111-1111-1111-aaaaaaaa0021",
    "LA Clippers":            "11111111-1111-1111-1111-aaaaaaaa0022",
    "Los Angeles Lakers":     "11111111-1111-1111-1111-aaaaaaaa0023",
    "Phoenix Suns":           "11111111-1111-1111-1111-aaaaaaaa0024",
    "Sacramento Kings":       "11111111-1111-1111-1111-aaaaaaaa0025",
    "Dallas Mavericks":       "11111111-1111-1111-1111-aaaaaaaa0026",
    "Houston Rockets":        "11111111-1111-1111-1111-aaaaaaaa0027",
    "Memphis Grizzlies":      "11111111-1111-1111-1111-aaaaaaaa0028",
    "New Orleans Pelicans":   "11111111-1111-1111-1111-aaaaaaaa0029",
    "San Antonio Spurs":      "11111111-1111-1111-1111-aaaaaaaa0030",
}

AGENCY_NAME_TO_ID = {
    "Billy the Kid Sports Agency": "a9e0c111-0000-0000-0000-000000000001",
    "The Boys Sports Agency":      "a9e0c222-0000-0000-0000-000000000002",
}


def sql_str(s):
    return "'" + s.replace("'", "''") + "'"


def derive_username(email):
    """Email prefix, sanitized to a-zA-Z0-9 plus . _ -"""
    prefix = email.split("@", 1)[0]
    return re.sub(r"[^A-Za-z0-9._-]", "_", prefix)


def main():
    # Resolve user IDs. For each email not in EXISTING_USER_IDS, mint a new UUID.
    user_ids = dict(EXISTING_USER_IDS)
    new_users = []  # list of dicts to be INSERTed into fantasy_db_staging

    for _, email, _ in OWNERSHIP:
        if email in user_ids:
            continue
        new_id = str(uuid.uuid4())
        user_ids[email] = new_id
        new_users.append({
            "id":       new_id,
            "email":    email,
            "username": derive_username(email),
        })

    # ----- Output 1: user inserts (fantasy_db_staging) -----
    with open("/tmp/seed_users.sql", "w", encoding="utf-8", newline="\n") as f:
        f.write("-- Generated by scripts/seed_nba_owners.py\n")
        f.write("-- Apply against fantasy_db_staging\n")
        f.write("BEGIN;\n\n")
        for u in new_users:
            f.write(
                f"INSERT INTO users (id, email, username, password_hash, role) VALUES "
                f"('{u['id']}', {sql_str(u['email'])}, {sql_str(u['username'])}, "
                f"{sql_str(PLACEHOLDER_HASH)}, 'user') "
                f"ON CONFLICT (email) DO NOTHING;\n"
            )
        f.write(f"\n-- {len(new_users)} new placeholder users\n")
        f.write("COMMIT;\n")
    print(f"Wrote /tmp/seed_users.sql  ({len(new_users)} new users)")

    # ----- Output 2: ownership inserts (fantasy_basketball_db_staging) -----
    with open("/tmp/seed_ownership.sql", "w", encoding="utf-8", newline="\n") as f:
        f.write("-- Generated by scripts/seed_nba_owners.py\n")
        f.write("-- Apply against fantasy_basketball_db_staging\n")
        f.write("BEGIN;\n\n")
        team_count = 0
        agency_count = 0
        unmatched = []
        for name, email, kind in OWNERSHIP:
            uid = user_ids[email]
            if kind == "team":
                tid = TEAM_NAME_TO_ID.get(name)
                if not tid:
                    unmatched.append((name, email))
                    continue
                f.write(
                    f"INSERT INTO team_owners (team_id, user_id, is_primary) "
                    f"VALUES ('{tid}', '{uid}', TRUE) "
                    f"ON CONFLICT (team_id, user_id) DO NOTHING;  -- {name} : {email}\n"
                )
                team_count += 1
            elif kind == "agency":
                aid = AGENCY_NAME_TO_ID.get(name)
                if not aid:
                    unmatched.append((name, email))
                    continue
                f.write(
                    f"INSERT INTO agency_members (agency_id, user_id, is_primary) "
                    f"VALUES ('{aid}', '{uid}', TRUE) "
                    f"ON CONFLICT (agency_id, user_id) DO NOTHING;  -- {name} : {email}\n"
                )
                agency_count += 1
        f.write(f"\n-- {team_count} team_owners, {agency_count} agency_members\n")
        f.write("COMMIT;\n")
        if unmatched:
            print("UNMATCHED:", unmatched, file=sys.stderr)
    print(f"Wrote /tmp/seed_ownership.sql  ({team_count} team_owners, {agency_count} agency_members)")


if __name__ == "__main__":
    main()
