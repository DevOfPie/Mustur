"""Turn arbitrary titles into URL-safe slugs."""
import re
import unicodedata


def slugify(text: str, max_length: int = 60) -> str:
    text = unicodedata.normalize("NFKD", text)
    text = text.encode("ascii", "ignore").decode("ascii")
    text = re.sub(r"[^a-zA-Z0-9]+", "-", text).strip("-").lower()
    return text[:max_length].rstrip("-")


def unique_slug(text: str, existing: set) -> str:
    base = slugify(text)
    if base not in existing:
        return base
    n = 2
    while f"{base}-{n}" in existing:
        n += 1
    return f"{base}-{n}"
