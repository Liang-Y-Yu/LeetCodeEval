#!/usr/bin/env python3
"""
Simplified pattern analysis without external dependencies
"""

import os
import re
import json
from collections import defaultdict, Counter

class SimplePatternExtractor:
    def __init__(self):
        self.patterns = {
            'array_manipulation': [
                r'for\s*\([^)]*\s*:\s*\w+\[\]',
                r'\w+\[\s*\w+\s*\]\s*=',
                r'Arrays\.(sort|binarySearch|copyOf)',
                r'new\s+\w+\[\s*\w*\s*\]'
            ],
            'string_processing': [
                r'String\w*\.(substring|indexOf|replace|split|matches)',
                r'StringBuilder\s*\w+\s*=\s*new\s*StringBuilder',
                r'Pattern\.(compile|matches)',
                r'\.toString\(\)'
            ],
            'hash_map_operations': [
                r'HashMap<[^>]+>\s*\w+\s*=\s*new\s*HashMap',
                r'Map<[^>]+>\s*\w+',
                r'\w+\.put\s*\(',
                r'\w+\.get\s*\(',
                r'\w+\.containsKey\s*\('
            ],
            'mathematical_computation': [
                r'Math\.(max|min|abs|pow|sqrt)',
                r'BigDecimal\s+\w+',
                r'Random\s+\w+\s*=\s*new',
                r'calculate\w*\s*\(',
                r'compute\w*\s*\('
            ],
            'sorting_searching': [
                r'Collections\.sort\s*\(',
                r'Arrays\.sort\s*\(',
                r'Comparator<[^>]+>',
                r'\.stream\(\).*\.sorted\(',
                r'binarySearch'
            ],
            'validation_logic': [
                r'validate\w*\s*\(',
                r'if\s*\([^)]*null[^)]*\)',
                r'throw\s+new\s+\w*Exception',
                r'assert\w*\s*\('
            ]
        }
        
        self.leetcode_mapping = {
            'array_manipulation': ['Two Sum', 'Best Time to Buy and Sell Stock', 'Rotate Array'],
            'string_processing': ['Valid Anagram', 'Longest Substring Without Repeating Characters', 'Valid Parentheses'],
            'hash_map_operations': ['Two Sum', 'Group Anagrams', 'Top K Frequent Elements'],
            'mathematical_computation': ['Pow(x, n)', 'Sqrt(x)', 'Happy Number'],
            'sorting_searching': ['Merge Intervals', 'Search in Rotated Sorted Array', 'Kth Largest Element'],
            'validation_logic': ['Valid Parentheses', 'Valid Sudoku', 'Valid Binary Search Tree']
        }

    def extract_from_file(self, filepath):
        try:
            with open(filepath, 'r', encoding='utf-8', errors='ignore') as f:
                content = f.read()
            
            results = []
            methods = self._extract_methods(content)
            
            for method_name, method_code in methods.items():
                for pattern_type, regexes in self.patterns.items():
                    matches = 0
                    for regex in regexes:
                        if re.search(regex, method_code, re.IGNORECASE):
                            matches += 1
                    
                    if matches > 0:
                        confidence = min(0.3 + (matches * 0.2), 1.0)
                        business_context = self._get_business_context(filepath)
                        
                        results.append({
                            'file': filepath,
                            'method': method_name,
                            'pattern_type': pattern_type,
                            'confidence': confidence,
                            'matches': matches,
                            'business_context': business_context,
                            'leetcode_problems': self.leetcode_mapping.get(pattern_type, [])[:3],
                            'code_snippet': method_code[:200] + '...' if len(method_code) > 200 else method_code
                        })
            
            return results
        except Exception as e:
            return []

    def _extract_methods(self, java_code):
        methods = {}
        # Simple method extraction
        method_pattern = r'(public|private|protected)?\s*(static)?\s*\w+\s+(\w+)\s*\([^)]*\)\s*\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}'
        matches = re.finditer(method_pattern, java_code, re.MULTILINE | re.DOTALL)
        
        for match in matches:
            method_name = match.group(3)
            method_body = match.group(4)
            if len(method_body.strip()) > 20:  # Skip trivial methods
                methods[method_name] = method_body
        
        return methods

    def _get_business_context(self, filepath):
        business_keywords = ['invoice', 'payment', 'balance', 'account', 'transaction', 
                           'settlement', 'card', 'loan', 'savings', 'transfer', 'financial']
        
        path_lower = filepath.lower()
        contexts = [kw for kw in business_keywords if kw in path_lower]
        return ', '.join(contexts) if contexts else 'general'

def analyze_repository():
    print("🔍 Starting simplified pattern analysis...")
    
    extractor = SimplePatternExtractor()
    repo_path = "/local/repos/mcom-m3"
    
    all_patterns = []
    processed_files = 0
    
    for root, dirs, files in os.walk(repo_path):
        # Skip build and test directories
        if any(skip in root for skip in ['/build/', '/target/', '/.git/', '/test/']):
            continue
            
        for file in files:
            if file.endswith('.java') and not file.endswith('Test.java'):
                filepath = os.path.join(root, file)
                patterns = extractor.extract_from_file(filepath)
                all_patterns.extend(patterns)
                processed_files += 1
                
                if processed_files % 50 == 0:
                    print(f"   Processed {processed_files} files, found {len(all_patterns)} patterns")
    
    print(f"✅ Analysis complete: {processed_files} files processed")
    print(f"📊 Found {len(all_patterns)} algorithmic patterns")
    
    # Generate statistics
    pattern_stats = Counter(p['pattern_type'] for p in all_patterns)
    business_stats = Counter(p['business_context'] for p in all_patterns)
    confidence_stats = {
        'high': sum(1 for p in all_patterns if p['confidence'] > 0.7),
        'medium': sum(1 for p in all_patterns if 0.5 <= p['confidence'] <= 0.7),
        'low': sum(1 for p in all_patterns if p['confidence'] < 0.5)
    }
    
    # Create results directory
    os.makedirs('results', exist_ok=True)
    
    # Save detailed results
    with open('results/patterns.json', 'w') as f:
        json.dump(all_patterns, f, indent=2)
    
    # Create summary
    summary = {
        'total_patterns': len(all_patterns),
        'files_processed': processed_files,
        'pattern_distribution': dict(pattern_stats),
        'business_context_distribution': dict(business_stats),
        'confidence_distribution': confidence_stats,
        'top_patterns_by_confidence': sorted(all_patterns, key=lambda x: x['confidence'], reverse=True)[:10]
    }
    
    with open('results/summary.json', 'w') as f:
        json.dump(summary, f, indent=2)
    
    # Print results
    print(f"\n📈 Pattern Distribution:")
    for pattern, count in pattern_stats.most_common():
        print(f"   {pattern}: {count}")
    
    print(f"\n🏢 Business Context Distribution:")
    for context, count in business_stats.most_common(5):
        print(f"   {context}: {count}")
    
    print(f"\n🎯 Confidence Distribution:")
    for level, count in confidence_stats.items():
        print(f"   {level}: {count}")
    
    print(f"\n🔗 Top LeetCode Mappings Found:")
    leetcode_problems = set()
    for pattern in all_patterns:
        leetcode_problems.update(pattern['leetcode_problems'])
    
    for problem in list(leetcode_problems)[:10]:
        print(f"   - {problem}")
    
    return all_patterns

if __name__ == "__main__":
    patterns = analyze_repository()
    print(f"\n✨ Results saved to 'results/' directory")
    print(f"📄 Key files:")
    print(f"   - results/patterns.json (detailed patterns)")
    print(f"   - results/summary.json (statistics)")
