"""Remove background from an image using rembg (U2-Net model).

Usage: reads PNG from stdin, writes PNG (with transparent background) to stdout.
"""
import sys
from rembg import remove

data = sys.stdin.buffer.read()
result = remove(data)
sys.stdout.buffer.write(result)
