#!/usr/bin/env python3
import os
import re
import json
import glob

class IndustryScanner:
    def __init__(self):
        self.patterns = {
            'hash_map': [
                r'HashMap<[^>]+>\s*\w+\s*=\s*new\s*HashMap',
                r'Map<[^>]+>\s*\w+',
                r'\w+\.put\s*\(',
                r'\w+\.get\s*\(',
                r'\w+\.containsKey\s*\('
            ],
            'sorting': [
                r'Collections\.sort\s*\(',
                r'Arrays\.sort\s*\(',
                r'\.stream\(\).*\.sorted\(',
                r'Comparator\.'
            ],
            'string_ops': [
                r'String\w*\.(substring|indexOf|replace|split|matches)',
                r'StringBuilder',
                r'Pattern\.compile'
            ],
            'tree_graph': [
                # Heuristic for recursion: method calling itself? Hard with regex.
                # We'll look for "Node", "Tree", "Graph" classes or "traverse" methods
                r'class\s+\w*Node',
                r'class\s+\w*Tree',
                r'traverse\w*\('
            ]
        }

    def scan_directory(self, root_dir):
        summary = {
            "total_projects": 0,
            "total_files": 0,
            "total_patterns": {k: 0 for k in self.patterns},
            "total_pattern_loc": 0,
            "projects": []
        }

        # Get subdirectories
        try:
            subdirs = [d for d in os.listdir(root_dir) if os.path.isdir(os.path.join(root_dir, d))]
        except FileNotFoundError:
            print(f"Directory not found: {root_dir}")
            return summary

        for i, subdir in enumerate(subdirs):
            project_id = f"Project {i+1}"
            project_path = os.path.join(root_dir, subdir)
            print(f"Scanning {subdir} as {project_id}...")

            proj_result = {
                "project_id": project_id,
                "total_files": 0,
                "patterns": {k: 0 for k in self.patterns},
                "pattern_loc": 0
            }

            for root, _, files in os.walk(project_path):
                for file in files:
                    if file.endswith(".java"):
                        proj_result["total_files"] += 1
                        summary["total_files"] += 1
                        self.scan_file(os.path.join(root, file), proj_result, summary)

            summary["projects"].append(proj_result)
            summary["total_projects"] += 1
            summary["total_patterns"] = {k: summary["total_patterns"][k] + proj_result["patterns"][k] for k in self.patterns}
            summary["total_pattern_loc"] += proj_result["pattern_loc"]

        return summary

    def scan_file(self, filepath, proj_result, summary):
        try:
            with open(filepath, 'r', encoding='utf-8', errors='ignore') as f:
                content = f.read()
                
            # Count patterns (matches)
            for pattern_type, regexes in self.patterns.items():
                for regex in regexes:
                    matches = len(re.findall(regex, content))
                    if matches > 0:
                        proj_result["patterns"][pattern_type] += matches
                        # Total patterns updated at project level to avoid race/complexity, 
                        # but here we just update project counts and aggregate later or pass refs.
                        # Actually, let's stick to the previous design but simpler:
                        # Update proj_result["patterns"] here.
            
            # Count Pattern LOC
            lines = content.splitlines()
            loc_count = 0
            for line in lines:
                for pattern_type, regexes in self.patterns.items():
                    found = False
                    for regex in regexes:
                        if re.search(regex, line):
                            loc_count += 1
                            found = True
                            break
                    if found:
                        break
            proj_result["pattern_loc"] += loc_count
            
        except Exception:
            pass

if __name__ == "__main__":
    scanner = IndustryScanner()
    results = scanner.scan_directory("industry-repos")
    
    os.makedirs("results/industry_scan", exist_ok=True)
    with open("results/industry_scan/summary.json", "w") as f:
        json.dump(results, f, indent=2)
    
    print(f"Scan complete. Found {results['total_files']} Java files in {results['total_projects']} projects.")
