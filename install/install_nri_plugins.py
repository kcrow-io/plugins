#!/usr/bin/env python3
import os
import sys
import shutil
import signal
import tempfile
import argparse
from pathlib import Path

def parse_args():
    parser = argparse.ArgumentParser(description='Install NRI plugins')
    
    # Get defaults from environment variables
    parser.add_argument('--nri-bin-path', 
                       default=os.getenv('KCROW_NRI_BIN_PATH', '/opt/nri/plugins'),
                       help='containerd nri plugins binary path (env: KCROW_NRI_BIN_PATH)')
    parser.add_argument('--nri-etc-path', 
                       default=os.getenv('KCROW_NRI_ETC_PATH', '/etc/nri/conf.d'),
                       help='containerd nri config path (env: KCROW_NRI_ETC_PATH)') 
    parser.add_argument('--bin-path', 
                       default=os.getenv('KCROW_BIN_PATH', '/opt/kcrow/bin'),
                       help='custom plugins binary path (env: KCROW_BIN_PATH)')
    parser.add_argument('--etc-path', 
                       default=os.getenv('KCROW_ETC_PATH', '/opt/kcrow/conf.d'),
                       help='custom plugins config path (env: KCROW_ETC_PATH)')
    parser.add_argument('--restart', 
                       action='store_true',
                       default=os.getenv('KCROW_RESTART', 'false').lower() == 'true',
                       help='send signal to restart containerd (env: KCROW_RESTART)')
    parser.add_argument('--loop', 
                       action='store_true',
                       default=os.getenv('KCROW_LOOP', 'false').lower() == 'true',
                       help='loop until exit signal received (env: KCROW_LOOP)')
    parser.add_argument('--install', 
                       default=os.getenv('KCROW_INSTALL', 'override,escape'),
                       help='plugins to install, comma separated (env: KCROW_INSTALL)')
    parser.add_argument('--begin-number',
                       type=int,
                       default=int(os.getenv('KCROW_BEGIN_NUMBER', '10')),
                       help='starting number for plugin ordering (env: KCROW_BEGIN_NUMBER)')
    return parser.parse_args()

def check_and_create_dir(dir_path):
    """Check if directory exists, create if not"""
    try:
        Path(dir_path).mkdir(parents=True, exist_ok=True)
    except Exception as e:
        print(f"Failed to create directory {dir_path}: {e}")
        sys.exit(1)

def is_executable(file_path):
    """Check if file is executable"""
    return os.access(file_path, os.X_OK)

def atomic_copy(src, dst):
    """Atomic file copy"""
    try:
        # Copy to temp file first
        temp_dst = f"{dst}.tmp"
        shutil.copy2(src, temp_dst)
        # Rename to target file
        os.rename(temp_dst, dst)
    except Exception as e:
        # Clean up temp file
        if os.path.exists(temp_dst):
            os.unlink(temp_dst)
        raise e

def copy_plugins(plugins, src_dir, dst_dir, etc_src_dir, etc_dst_dir, begin_number):
    """Copy plugin files with numbered prefixes and config files"""
    failed = False
    copied_files = []
    copied_configs = []
    
    try:
        print(f"Starting to copy {len(plugins)} plugins from {src_dir} to {dst_dir}")
        
        for i, plugin in enumerate(plugins):
            current_number = begin_number + i
            src = os.path.join(src_dir, plugin)
            dst = os.path.join(dst_dir, f"{current_number}-{plugin}")
            
            print(f"Processing plugin: {plugin} (will be {current_number}-{plugin})")
            
            # Check plugin binary
            if not os.path.exists(src):
                print(f"[ERROR] Plugin file {src} does not exist")
                failed = True
                break
                
            if not is_executable(src):
                print(f"[ERROR] Plugin file {src} is not executable")
                failed = True
                break
                
            # Check config file
            config_src = os.path.join(etc_src_dir, f"{plugin}.conf")
            config_dst = os.path.join(etc_dst_dir, f"{current_number}-{plugin}.conf")
            
            try:
                # Copy plugin binary
                print(f"Copying {src} to {dst}")
                atomic_copy(src, dst)
                copied_files.append(dst)
                
                # Copy config file if exists
                if os.path.exists(config_src):
                    print(f"Copying config {config_src} to {config_dst}")
                    atomic_copy(config_src, config_dst)
                    copied_configs.append(config_dst)
                else:
                    print(f"No config file found for {plugin}")
                    
                print(f"Successfully processed {plugin}")
                
            except Exception as e:
                print(f"[ERROR] Failed to process plugin {plugin}: {e}")
                failed = True
                break
                
    except Exception as e:
        failed = True
        print(f"[ERROR] Error during copy operation: {e}")
    
    # Clean up on failure
    if failed:
        print("Cleaning up due to failure...")
        for f in copied_files:
            try:
                print(f"Removing failed plugin copy: {f}")
                os.unlink(f)
            except Exception as e:
                print(f"[WARNING] Failed to remove plugin {f}: {e}")
        for f in copied_configs:
            try:
                print(f"Removing failed config copy: {f}")
                os.unlink(f)
            except Exception as e:
                print(f"[WARNING] Failed to remove config {f}: {e}")
        sys.exit(1)
    
    print(f"Successfully processed all {len(plugins)} plugins and their configs")

def find_containerd_pid():
    """Find containerd process PID"""
    for pid in os.listdir('/proc'):
        if not pid.isdigit():
            continue
            
        try:
            with open(f'/proc/{pid}/cmdline', 'rb') as f:
                cmdline = f.read().decode('utf-8').replace('\x00', ' ')
                if 'containerd ' in cmdline or '/containerd ' in cmdline:
                    return int(pid)
        except:
            continue
            
    return None

def send_sighup_to_containerd():
    """Send SIGHUP to containerd"""
    pid = find_containerd_pid()
    if pid:
        try:
            os.kill(pid, signal.SIGHUP)
            print(f"Sent SIGHUP to containerd process (PID: {pid})")
        except Exception as e:
            print(f"Failed to send SIGHUP to containerd: {e}")
    else:
        print("Containerd process not found")

def main():
    args = parse_args()
    
    print("Starting NRI plugin installation...")
    print(f"Configuration:")
    print(f"  Source bin path: {args.bin_path}")
    print(f"  Target bin path: {args.nri_bin_path}")
    print(f"  Plugins to install: {args.install}")
    print(f"  Begin number: {args.begin_number}")
    print(f"  Restart containerd: {args.restart}")
    print(f"  Loop mode: {args.loop}")
    
    # Check and create directories
    print("\nChecking/creating directories...")
    check_and_create_dir(args.nri_bin_path)
    check_and_create_dir(args.nri_etc_path)
    print("Directory check completed successfully")
    
    # Copy plugins
    print("\nStarting plugin copy process...")
    plugins = args.install.split(',')
    copy_plugins(plugins, args.bin_path, args.nri_bin_path, args.etc_path, args.nri_etc_path, args.begin_number)
    
    # Restart containerd if needed
    if args.restart:
        print("\nAttempting to restart containerd...")
        send_sighup_to_containerd()
    else:
        print("\nSkipping containerd restart (not requested)")
    
    # Wait for exit signal if in loop mode
    if args.loop:
        print("\nEntering loop mode, waiting for exit signal...")
        signal.pause()
        print("Exit signal received, shutting down...")
    else:
        print("\nInstallation completed successfully")

if __name__ == '__main__':
    main()
