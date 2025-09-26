"""Image caching functionality for the roxie deployment tool."""

import json
import os
import subprocess

from rich.console import Console

from logger import Logger


class ImageCache:
    """Manages cache of verified pullable Docker images"""

    def __init__(self, logger: Logger, cache_file: str | None = None, max_entries: int = 20):
        """Initialize ImageCache with optional cache file path and max entries"""
        self.cache_file = cache_file or os.path.expanduser("~/.roxie.image_cache")
        self.max_entries = max_entries
        self._cache = self.load_cache()
        self.logger = logger

    def load_cache(self) -> list[str]:
        """Load image cache from file"""
        try:
            if os.path.exists(self.cache_file):
                with open(self.cache_file) as f:
                    data = json.load(f)
                    # Handle both old format (list) and new format (dict with 'images' key)
                    if isinstance(data, list):
                        return data
                    elif isinstance(data, dict):
                        images_value = data.get("images", [])
                        # Ensure we always return a list[str]
                        return [str(x) for x in images_value]
        except (json.JSONDecodeError, OSError):
            pass
        return []

    def save_cache(self):
        """Save image cache to file"""
        try:
            # Ensure directory exists
            os.makedirs(os.path.dirname(self.cache_file), exist_ok=True)

            with open(self.cache_file, "w") as f:
                json.dump({"images": self._cache}, f, indent=2)
        except OSError:
            # Silently fail on cache save errors
            pass

    def is_cached(self, image_ref: str) -> bool:
        """Check if image is in cache"""
        return image_ref in self._cache

    def add_to_cache(self, image_ref: str):
        """Add image to cache, maintaining max_entries limit"""
        if image_ref in self._cache:
            # Move to end (most recent)
            self._cache.remove(image_ref)

        self._cache.append(image_ref)

        # Maintain max entries
        if len(self._cache) > self.max_entries:
            self._cache = self._cache[-self.max_entries :]

        self.save_cache()

    def test_cache_functionality(self, console: Console):
        """Test cache read/write functionality"""
        try:
            # Test write
            test_image = "test/image:latest"
            original_cache = self._cache.copy()
            self.add_to_cache(test_image)

            # Test read
            if self.is_cached(test_image):
                console.print("✓ Cache test passed", style="dim green")
            else:
                console.print("⚠ Cache test failed", style="dim yellow")

            # Restore original cache
            self._cache = original_cache
            self.save_cache()
        except Exception as e:
            console.print(f"Cache test failed: {str(e)}", style="dim yellow")

    def verify_image_pullable(self, image_ref: str) -> bool:
        """Verify if image is pullable, using cache when possible"""
        if self.is_cached(image_ref):
            return True

        # Use skopeo to check if image is pullable
        try:
            result = subprocess.run(
                ["skopeo", "inspect", "--raw", f"docker://{image_ref}"], capture_output=True, text=True, timeout=30
            )

            if result.returncode == 0:
                self.add_to_cache(image_ref)
                return True
            else:
                print(result.stderr)
                print(result.stdout)

        except (subprocess.TimeoutExpired, subprocess.CalledProcessError, FileNotFoundError):
            pass

        return False

    def verify_images_pullable(self, *images: str) -> bool:
        """Verify that Docker images are pullable using cached results for speed"""
        if not images:
            return True

        # Skip verification if environment variable is set
        if os.environ.get("SKIP_IMAGE_VERIFICATION", "").lower() in ("true", "1", "yes"):
            self.logger.print_with_timestamp(
                f"Skipping image verification for {len(images)} images (SKIP_IMAGE_VERIFICATION=true)"
            )
            return True

        # Separate cached and uncached images
        cached_images = []
        uncached_images = []

        for img in images:
            if self.is_cached(img):
                cached_images.append(img)
            else:
                uncached_images.append(img)

        # Report cached results immediately
        if cached_images:
            self.logger.print_with_timestamp(f"✓ {len(cached_images)} images verified from cache", style="dim green")
            for img in cached_images:
                self.logger.print_with_timestamp(f"✓ Image {img} (cached)", style="dim green")

        # Verify uncached images if any
        failed_images = []
        if uncached_images:
            import concurrent.futures

            def verify_single_image(img: str) -> tuple[str, bool, str]:
                """Verify a single image using ImageCache"""
                if self.verify_image_pullable(img):
                    return (img, True, "")
                else:
                    return (img, False, "not pullable")

            # Verify uncached images in parallel (max 4 concurrent to avoid overwhelming registry)
            max_workers = min(4, len(uncached_images))

            with concurrent.futures.ThreadPoolExecutor(max_workers=max_workers) as executor:
                # Submit all verification tasks
                future_to_image = {executor.submit(verify_single_image, img): img for img in uncached_images}

                # Collect results as they complete
                for future in concurrent.futures.as_completed(future_to_image):
                    img, success, error_msg = future.result()

                    if success:
                        self.logger.print_with_timestamp(f"✓ Image {img} verified", style="dim green")
                    else:
                        self.logger.print_with_timestamp(f"✗ Image {img} failed: {error_msg}", style="bold red")
                        failed_images.append((img, error_msg))

        if failed_images:
            self.logger.error(f"Failed to verify {len(failed_images)} images:")
            for img, error_msg in failed_images:
                self.logger.error(f"  - {img}: {error_msg}")
            return False

        total_images = len(images)
        cached_count = len(cached_images)
        verified_count = len(uncached_images) - len(failed_images)

        self.logger.print_with_timestamp(
            f"✓ All {total_images} images verified successfully ({cached_count} cached, {verified_count} verified)",
            style="bold green",
        )
        return True
