import json

from image_cache import ImageCache
from logger import Logger


def test_image_cache_load_save_roundtrip(tmp_path):
    cache_path = tmp_path / ".roxie.image_cache"
    logger = Logger()
    c = ImageCache(logger, cache_file=str(cache_path))
    assert c._cache == []

    c.add_to_cache("quay.io/example/app:1")
    assert c.is_cached("quay.io/example/app:1")

    # Reopen and verify persistence
    c2 = ImageCache(logger, cache_file=str(cache_path))
    assert c2.is_cached("quay.io/example/app:1")


def test_image_cache_handles_old_format(tmp_path):
    cache_path = tmp_path / ".roxie.image_cache"
    cache_path.write_text(json.dumps(["a", "b"]))

    logger = Logger()
    c = ImageCache(logger, cache_file=str(cache_path))
    assert c.is_cached("a")
    assert c.is_cached("b")
