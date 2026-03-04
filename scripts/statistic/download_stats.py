#!/usr/bin/env python3
import json
import subprocess
import sys
from datetime import datetime

def format_number(n):
    return f"{n:,}"

def format_date(date_str):
    try:
        dt = datetime.fromisoformat(date_str.replace('Z', '+00:00'))
        return dt.strftime('%Y-%m-%d %H:%M')
    except Exception:
        return date_str

def fetch_releases():
    """Fetch releases JSON via curl (uses system CA store; works on macOS and elsewhere)."""
    url = "https://api.github.com/repos/Leadaxe/singbox-launcher/releases"
    try:
        result = subprocess.run(
            [
                "curl", "-sS", "-L",
                "-H", "Accept: application/vnd.github.v3+json",
                "-H", "User-Agent: singbox-launcher/1.0",
                "--connect-timeout", "15", "--max-time", "30",
                url,
            ],
            capture_output=True,
            text=True,
            check=True,
        )
        return json.loads(result.stdout)
    except FileNotFoundError:
        print("Error: curl is required. Install curl or run on a system with curl.", file=sys.stderr)
        sys.exit(1)
    except subprocess.CalledProcessError as e:
        print(f"Error: curl failed: {e.stderr or e}", file=sys.stderr)
        sys.exit(1)
    except json.JSONDecodeError as e:
        print(f"Error parsing JSON: {e}", file=sys.stderr)
        sys.exit(1)

def main():
    try:
        releases = fetch_releases()
    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        sys.exit(1)
    
    if not releases:
        print("No releases found")
        return
    
    # Process releases
    release_stats = []
    total_downloads = 0
    
    for release in releases:
        tag = release.get('tag_name', 'N/A')
        published = release.get('published_at', 'N/A')
        assets = release.get('assets', [])
        
        # Calculate total downloads for this release
        release_total = sum(asset.get('download_count', 0) for asset in assets)
        total_downloads += release_total
        
        # Count assets
        win_count = sum(1 for a in assets if 'win' in a.get('name', '').lower())
        mac_count = sum(1 for a in assets if 'macos' in a.get('name', '').lower())
        
        release_stats.append({
            'version': tag,
            'date': format_date(published),
            'downloads': release_total,
            'assets_count': len(assets),
            'win': win_count > 0,
            'mac': mac_count > 0
        })
    
    # Sort by version (newest first)
    release_stats.sort(key=lambda x: x['version'], reverse=True)
    
    # Print table
    print("=" * 90)
    print("📊 Download Statistics for Leadaxe/singbox-launcher")
    print("=" * 90)
    print()
    
    # Table header
    print(f"{'Version':<12} {'Release Date':<18} {'Downloads':>12} {'Assets':>8} {'Platforms':<15}")
    print("-" * 90)
    
    # Table rows
    for stat in release_stats:
        platforms = []
        if stat['win']:
            platforms.append("Windows")
        if stat['mac']:
            platforms.append("macOS")
        platform_str = ", ".join(platforms) if platforms else "N/A"
        
        print(f"{stat['version']:<12} {stat['date']:<18} {format_number(stat['downloads']):>12} "
              f"{stat['assets_count']:>8} {platform_str:<15}")
    
    print("-" * 90)
    print(f"{'TOTAL':<12} {'':<18} {format_number(total_downloads):>12} {'':>8} {'':<15}")
    print("=" * 90)
    print()
    
    # Summary
    print("📈 Summary:")
    print(f"   Total releases: {len(releases)}")
    print(f"   Total downloads: {format_number(total_downloads)}")
    print(f"   Average downloads per release: {format_number(total_downloads // len(releases) if releases else 0)}")
    print()
    
    # Latest release (first in sorted list)
    if release_stats:
        latest = release_stats[0]
        print("=" * 90)
        print("🆕 Latest Release")
        print("=" * 90)
        print(f"   🏷️  Version:     {latest['version']}")
        print(f"   📅 Date:         {latest['date']}")
        print(f"   ⬇️  Downloads:    {format_number(latest['downloads'])}")
        print(f"   📦 Assets:       {latest['assets_count']}")
        platforms = []
        if latest['win']:
            platforms.append("Windows")
        if latest['mac']:
            platforms.append("macOS")
        platform_str = ", ".join(platforms) if platforms else "N/A"
        print(f"   💻 Platforms:    {platform_str}")
        print()
    
    # Top 3 releases
    top_releases = sorted(release_stats, key=lambda x: x['downloads'], reverse=True)[:3]
    print("=" * 90)
    print("🏆 Top 3 Releases by Downloads")
    print("=" * 90)
    medals = ["🥇", "🥈", "🥉"]
    for i, stat in enumerate(top_releases):
        medal = medals[i] if i < len(medals) else "  "
        print(f"{medal} {stat['version']:<12} {format_number(stat['downloads']):>12} downloads ({stat['date']})")
    print()

if __name__ == "__main__":
    main()

