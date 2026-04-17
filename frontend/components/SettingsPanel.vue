<template>
  <div class="relative">
    <!-- Gear icon button -->
    <button
      class="p-1.5 rounded-lg hover:bg-surface-700 transition-colors text-surface-400"
      title="Settings"
      @click="isOpen = !isOpen"
    >
      <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path
          stroke-linecap="round"
          stroke-linejoin="round"
          stroke-width="2"
          d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.066 2.573c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.573 1.066c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.066-2.573c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.573-1.066z"
        />
        <circle cx="12" cy="12" r="3" stroke-linecap="round" stroke-linejoin="round" stroke-width="2" />
      </svg>
      <!-- Recording indicator dot -->
      <span
        v-if="debugEnabled"
        class="absolute -top-0.5 -right-0.5 inline-flex rounded-full h-2 w-2 bg-green-500"
      />
    </button>

    <!-- Dropdown panel -->
    <div
      v-if="isOpen"
      class="absolute right-0 top-full mt-2 w-72 bg-surface-800 border border-surface-700 rounded-xl shadow-lg z-50 p-4"
    >
      <h3 class="text-xs font-semibold text-surface-300 uppercase tracking-wide mb-3">Debug Log</h3>

      <!-- Toggle -->
      <div class="flex items-center justify-between mb-3">
        <label class="text-sm text-surface-300">Record debug log</label>
        <button
          class="relative inline-flex h-5 w-9 rounded-full transition-colors"
          :class="debugEnabled ? 'bg-green-600' : 'bg-surface-600'"
          @click="toggleDebug"
        >
          <span
            class="inline-block h-4 w-4 rounded-full bg-white transform transition-transform mt-0.5"
            :class="debugEnabled ? 'translate-x-4 ml-0.5' : 'translate-x-0.5'"
          />
        </button>
      </div>

      <!-- Recording since -->
      <p v-if="debugEnabled && recordingSinceLabel" class="text-xs text-surface-400 mb-3">
        Recording since {{ recordingSinceLabel }}
      </p>

      <!-- Entry count -->
      <p v-if="debugEnabled" class="text-xs text-surface-400 mb-3">
        {{ entryCount }} entries captured
      </p>

      <!-- Download button -->
      <button
        v-if="debugEnabled"
        class="w-full px-3 py-2 text-xs rounded-lg bg-blue-600/30 text-blue-300 hover:bg-blue-600/50 transition-colors disabled:opacity-50"
        :disabled="isDownloading || entryCount === 0"
        @click="downloadDebugLog"
      >
        {{ isDownloading ? 'Preparing...' : 'Download debug log (.zip)' }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useDebugLog } from '~/composables/useDebugLog';
import type { DebugLogExport } from '~/composables/useDebugLog';

const { enable, disable, isEnabled, getRecordingSince, getEntryCount, exportLog } = useDebugLog();

const isOpen = ref(false);
const debugEnabled = ref(isEnabled());
const entryCount = ref(getEntryCount());
const isDownloading = ref(false);

// Update entry count periodically while open and enabled
let countInterval: ReturnType<typeof setInterval> | null = null;

watch(isOpen, (open) => {
  if (open) {
    debugEnabled.value = isEnabled();
    entryCount.value = getEntryCount();
    countInterval = setInterval(() => {
      entryCount.value = getEntryCount();
    }, 1000);
  } else {
    if (countInterval) {
      clearInterval(countInterval);
      countInterval = null;
    }
  }
});

onScopeDispose(() => {
  if (countInterval) clearInterval(countInterval);
});

const recordingSinceLabel = computed(() => {
  const since = getRecordingSince();
  if (!since) return null;
  return new Date(since).toLocaleTimeString();
});

function toggleDebug() {
  if (debugEnabled.value) {
    disable();
    debugEnabled.value = false;
    entryCount.value = 0;
  } else {
    enable();
    debugEnabled.value = true;
  }
}

async function downloadDebugLog() {
  isDownloading.value = true;
  try {
    const debugLog = exportLog();
    const sessionJson = buildSessionJson();
    await downloadAsZip(debugLog, sessionJson);
  } finally {
    isDownloading.value = false;
  }
}

function buildSessionJson(): Record<string, unknown> {
  return {
    exportedAt: new Date().toISOString(),
    url: window.location.href,
    userAgent: navigator.userAgent,
    viewport: {
      width: window.innerWidth,
      height: window.innerHeight,
      devicePixelRatio: window.devicePixelRatio,
    },
    connection: {
      type: (navigator as any).connection?.type ?? 'unknown',
      effectiveType: (navigator as any).connection?.effectiveType ?? 'unknown',
      downlink: (navigator as any).connection?.downlink ?? null,
    },
  };
}

/**
 * Create a zip archive containing debug-log.json and session.json,
 * then trigger a browser download.
 *
 * Uses the native CompressionStream API (no external dependencies).
 * Falls back to uncompressed JSON if CompressionStream is unavailable.
 */
async function downloadAsZip(debugLog: DebugLogExport, sessionInfo: Record<string, unknown>) {
  const timestamp = formatTimestamp(new Date());
  const filename = `voilot-debug-${timestamp}`;

  // Check for CompressionStream support
  if (typeof CompressionStream === 'undefined') {
    // Fallback: download as plain JSON
    const blob = new Blob(
      [JSON.stringify({ debugLog, session: sessionInfo }, null, 2)],
      { type: 'application/json' },
    );
    triggerDownload(blob, `${filename}.json`);
    return;
  }

  // Build a minimal zip archive
  const files = [
    { name: 'debug-log.json', data: JSON.stringify(debugLog, null, 2) },
    { name: 'session.json', data: JSON.stringify(sessionInfo, null, 2) },
  ];

  const zipBlob = await createZipBlob(files);
  triggerDownload(zipBlob, `${filename}.zip`);
}

/**
 * Create a zip blob from an array of {name, data} entries.
 * Uses the DEFLATE algorithm via CompressionStream.
 */
async function createZipBlob(files: Array<{ name: string; data: string }>): Promise<Blob> {
  const entries: Array<{
    name: Uint8Array;
    compressed: Uint8Array;
    uncompressed: Uint8Array;
    crc32: number;
    offset: number;
  }> = [];

  let offset = 0;

  for (const file of files) {
    const encoder = new TextEncoder();
    const nameBytes = encoder.encode(file.name);
    const uncompressed = encoder.encode(file.data);
    const crc = crc32(uncompressed);

    // Compress using CompressionStream (DEFLATE raw)
    const compressed = await deflateRaw(uncompressed);

    entries.push({
      name: nameBytes,
      compressed,
      uncompressed,
      crc32: crc,
      offset,
    });

    // Local file header (30 bytes + name length) + compressed data
    offset += 30 + nameBytes.length + compressed.length;
  }

  // Build the zip file
  const centralDirOffset = offset;
  const parts: Uint8Array[] = [];

  // Local file headers + data
  for (const entry of entries) {
    const header = new ArrayBuffer(30);
    const view = new DataView(header);
    view.setUint32(0, 0x04034b50, true);     // Local file header signature
    view.setUint16(4, 20, true);              // Version needed
    view.setUint16(6, 0, true);               // General purpose flag
    view.setUint16(8, 8, true);               // Compression method (DEFLATE)
    view.setUint16(10, 0, true);              // Last mod time
    view.setUint16(12, 0, true);              // Last mod date
    view.setUint32(14, entry.crc32, true);    // CRC-32
    view.setUint32(18, entry.compressed.length, true);   // Compressed size
    view.setUint32(22, entry.uncompressed.length, true); // Uncompressed size
    view.setUint16(26, entry.name.length, true);         // File name length
    view.setUint16(28, 0, true);              // Extra field length

    parts.push(new Uint8Array(header));
    parts.push(entry.name);
    parts.push(entry.compressed);
  }

  // Central directory
  let centralDirSize = 0;
  for (const entry of entries) {
    const cdHeader = new ArrayBuffer(46);
    const view = new DataView(cdHeader);
    view.setUint32(0, 0x02014b50, true);      // Central directory signature
    view.setUint16(4, 20, true);              // Version made by
    view.setUint16(6, 20, true);              // Version needed
    view.setUint16(8, 0, true);               // General purpose flag
    view.setUint16(10, 8, true);              // Compression method (DEFLATE)
    view.setUint16(12, 0, true);              // Last mod time
    view.setUint16(14, 0, true);              // Last mod date
    view.setUint32(16, entry.crc32, true);    // CRC-32
    view.setUint32(20, entry.compressed.length, true);   // Compressed size
    view.setUint32(24, entry.uncompressed.length, true); // Uncompressed size
    view.setUint16(28, entry.name.length, true);         // File name length
    view.setUint16(30, 0, true);              // Extra field length
    view.setUint16(32, 0, true);              // File comment length
    view.setUint16(34, 0, true);              // Disk number start
    view.setUint16(36, 0, true);              // Internal file attributes
    view.setUint32(38, 0, true);              // External file attributes
    view.setUint32(42, entry.offset, true);   // Relative offset of local header

    const cdBytes = new Uint8Array(cdHeader);
    parts.push(cdBytes);
    parts.push(entry.name);
    centralDirSize += 46 + entry.name.length;
  }

  // End of central directory record
  const eocd = new ArrayBuffer(22);
  const eocdView = new DataView(eocd);
  eocdView.setUint32(0, 0x06054b50, true);                // EOCD signature
  eocdView.setUint16(4, 0, true);                          // Disk number
  eocdView.setUint16(6, 0, true);                          // Disk with central dir
  eocdView.setUint16(8, entries.length, true);              // Entries on this disk
  eocdView.setUint16(10, entries.length, true);             // Total entries
  eocdView.setUint32(12, centralDirSize, true);             // Central dir size
  eocdView.setUint32(16, centralDirOffset, true);           // Central dir offset
  eocdView.setUint16(20, 0, true);                          // Comment length
  parts.push(new Uint8Array(eocd));

  return new Blob(parts, { type: 'application/zip' });
}

/**
 * Deflate raw (no zlib header) using CompressionStream.
 */
async function deflateRaw(data: Uint8Array): Promise<Uint8Array> {
  const stream = new CompressionStream('deflate-raw');
  const writer = stream.writable.getWriter();
  writer.write(data);
  writer.close();

  const reader = stream.readable.getReader();
  const chunks: Uint8Array[] = [];
  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    chunks.push(value);
  }

  const totalLength = chunks.reduce((sum, c) => sum + c.length, 0);
  const result = new Uint8Array(totalLength);
  let offset = 0;
  for (const chunk of chunks) {
    result.set(chunk, offset);
    offset += chunk.length;
  }
  return result;
}

/**
 * CRC-32 computation (standard IEEE polynomial).
 */
function crc32(data: Uint8Array): number {
  let crc = 0xFFFFFFFF;
  for (let i = 0; i < data.length; i++) {
    crc ^= data[i];
    for (let j = 0; j < 8; j++) {
      crc = (crc >>> 1) ^ (crc & 1 ? 0xEDB88320 : 0);
    }
  }
  return (crc ^ 0xFFFFFFFF) >>> 0;
}

function formatTimestamp(date: Date): string {
  const y = date.getFullYear();
  const m = String(date.getMonth() + 1).padStart(2, '0');
  const d = String(date.getDate()).padStart(2, '0');
  const h = String(date.getHours()).padStart(2, '0');
  const min = String(date.getMinutes()).padStart(2, '0');
  const s = String(date.getSeconds()).padStart(2, '0');
  return `${y}-${m}-${d}-${h}${min}${s}`;
}

function triggerDownload(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
}

// Close panel when clicking outside
if (import.meta.client) {
  const handleClickOutside = (e: MouseEvent) => {
    const el = (e.target as HTMLElement).closest('.relative');
    if (!el) {
      isOpen.value = false;
    }
  };
  onMounted(() => document.addEventListener('click', handleClickOutside));
  onUnmounted(() => document.removeEventListener('click', handleClickOutside));
}
</script>
