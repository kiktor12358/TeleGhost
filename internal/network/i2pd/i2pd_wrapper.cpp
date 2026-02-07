/*
 * C wrapper for i2pd C++ API
 * This file provides C-compatible functions that can be called via CGO
 *
 * Build with:
 *   g++ -std=c++17 -c i2pd_wrapper.cpp -I../i2pd/libi2pd -I../i2pd/i18n
 * -I../i2pd
 */

#include <atomic>
#include <chrono>
#include <iostream>
#include <string>
#include <thread>
#include <vector>

// i2pd includes
#include "Config.h"
#include "FS.h"
#include "Log.h"
#include "RouterContext.h"
#include "api.h"
#include "libi2pd_client/ClientContext.h"

// Hardcoded reseed URLs for better bootstrapping
const char *RESEED_URLS = "https://reseed.i2p-projekt.de/,"
                          "https://i2p.mooo.com/netDb/,"
                          "https://reseed.i2p.net/,"
                          "https://reseed-proxy.i2p.online/,"
                          "https://reseed.diva.exchange/";

static std::atomic<bool> g_running{false};
static std::string g_b32_address;
static std::string g_datadir;

extern "C" {

// Initialize i2pd with configuration
void i2pd_init(const char *datadir, int sam_enabled, int sam_port,
               int debug_mode) {
  g_datadir = datadir ? datadir : ".i2pd";

  // Force set data and certs directories in the filesystem helper
  // This ensures i2pd knows where to find its files even if command line
  // parsing fails
  i2p::fs::DetectDataDir(g_datadir, false);

  // Storage for strings to ensure they stay alive until InitI2P call
  static std::vector<std::string> args_storage;
  args_storage.clear();

  // Prepare arguments for i2pd
  args_storage.push_back("teleghost"); // argv[0]

  args_storage.push_back("--datadir");
  args_storage.push_back(g_datadir);

  args_storage.push_back("--certsdir");
  args_storage.push_back(g_datadir + "/certificates");

  if (sam_enabled) {
    args_storage.push_back("--sam.enabled");
    args_storage.push_back("true");
    args_storage.push_back("--sam.address");
    args_storage.push_back(
        "0.0.0.0"); // Bind to all interfaces to avoid localhost issues
    args_storage.push_back("--sam.port");
    args_storage.push_back(std::to_string(sam_port));
  } else {
    args_storage.push_back("--sam.enabled");
    args_storage.push_back("false");
  }

  // Set reseed to true just in case
  args_storage.push_back("--reseed.verify");
  args_storage.push_back("false");

  // Optimize for speed
  args_storage.push_back("--bandwidth");
  args_storage.push_back("X"); // Unlimited/High bandwidth

  args_storage.push_back("--tunconf.inbound.quantity");
  args_storage.push_back("3");
  args_storage.push_back("--tunconf.outbound.quantity");
  args_storage.push_back("3");
  args_storage.push_back("--tunconf.inbound.length");
  args_storage.push_back("2");
  args_storage.push_back("--tunconf.outbound.length");
  args_storage.push_back("2");

  // Robust bootstrapping
  args_storage.push_back("--reseed.urls");
  args_storage.push_back(RESEED_URLS);

  // Enable UPnP for better connectivity behind NAT
  args_storage.push_back("--upnp.enabled");
  args_storage.push_back("false");

  // Disable things we don't need to speed up
  args_storage.push_back("--http.enabled");
  args_storage.push_back("false");
  args_storage.push_back("--httpproxy.enabled");
  args_storage.push_back("false");
  args_storage.push_back("--socksproxy.enabled");
  args_storage.push_back("false");
  args_storage.push_back("--ircproxy.enabled");
  args_storage.push_back("false");

  // Logging configuration based on debug mode
  if (debug_mode) {
    args_storage.push_back("--log");
    args_storage.push_back("stdout");
    args_storage.push_back("--loglevel");
    args_storage.push_back("debug");
  } else {
    // Minimal logging in release mode
    args_storage.push_back("--log");
    args_storage.push_back(
        "none"); // Or file if needed, but user asked to not write logs
    args_storage.push_back("--loglevel");
    args_storage.push_back("error");
  }

  // Create argv pointers
  std::vector<char *> args_ptrs;
  if (debug_mode) {
    std::cout << "DEBUG: i2pd args (" << args_storage.size()
              << "):" << std::endl;
    for (auto &arg : args_storage) {
      args_ptrs.push_back(const_cast<char *>(arg.c_str()));
      std::cout << "  " << arg << std::endl;
    }
    std::cout << "-----------------" << std::endl;
  } else {
    for (auto &arg : args_storage) {
      args_ptrs.push_back(const_cast<char *>(arg.c_str()));
    }
  }

  // Initialize i2pd
  i2p::api::InitI2P(args_ptrs.size(), args_ptrs.data(), "TeleGhost");

  // Force start logging
  i2p::log::Logger().Start();
  if (debug_mode) {
    std::cout << "[i2pd_wrapper] I2P initialized." << std::endl;
  }

  if (debug_mode) {
    // TEST DEBUG: Verify configuration
    bool samEnabledConfig = false;
    i2p::config::GetOption("sam.enabled", samEnabledConfig);
    std::cout << "[i2pd_wrapper] TEST LOG: sam.enabled = "
              << (samEnabledConfig ? "true" : "false") << std::endl;

    std::string samAddr;
    i2p::config::GetOption("sam.address", samAddr);
    std::cout << "[i2pd_wrapper] TEST LOG: sam.address = " << samAddr
              << std::endl;

    uint16_t samPortConfig = 0;
    i2p::config::GetOption("sam.port", samPortConfig);
    std::cout << "[i2pd_wrapper] TEST LOG: sam.port = " << samPortConfig
              << std::endl;
  }
}

// Start i2pd router
void i2pd_start() {
  if (g_running)
    return;

  std::cout << "[i2pd_wrapper] Starting I2P router..." << std::endl;

  // Start the core router
  // api::StartI2P already calls client::context.Start()
  i2p::api::StartI2P(nullptr);
  i2p::client::context.Start();

  g_running = true;

  // Wait a bit for threads to spin up and SAM to initialize
  std::cout << "[i2pd_wrapper] Log: Waiting for SAM startup..." << std::endl;

  // Try waiting up to 10 seconds for SAM to become active
  for (int i = 0; i < 20; i++) {
    if (i2p::client::context.GetSAMBridge()) {
      std::cout << "[i2pd_wrapper] SAM Bridge is active after " << (i * 0.5)
                << "s." << std::endl;
      return;
    }
    std::this_thread::sleep_for(std::chrono::milliseconds(500));
  }

  // Final check
  if (i2p::client::context.GetSAMBridge()) {
    std::cout << "[i2pd_wrapper] SAM Bridge is active." << std::endl;
  } else {
    std::cout << "[i2pd_wrapper] WARNING: SAM Bridge NOT active after start "
                 "logic! Check logs."
              << std::endl;
  }
}

// Stop i2pd router
void i2pd_stop() {
  if (!g_running)
    return;

  i2p::client::context.Stop();
  i2p::api::StopI2P();
  g_running = false;
}

// Terminate and cleanup
void i2pd_terminate() {
  i2p::api::TerminateI2P();
  g_b32_address.clear();
}

// Check if router and client services are running
int i2pd_is_running() {
  if (!g_running)
    return 0;

  // Check if client context is started and SAM is available
  if (i2p::client::context.GetSAMBridge() != nullptr) {
    return 1;
  }

  return 0;
}

// Get router's B32 address
const char *i2pd_get_b32_address() {
  if (!g_running)
    return nullptr;

  try {
    auto &context = i2p::context;
    auto ident = context.GetRouterInfo().GetIdentHash();
    g_b32_address = ident.ToBase32() + ".b32.i2p";
    return g_b32_address.c_str();
  } catch (...) {
    return nullptr;
  }
}

} // extern "C"
