const storyList = document.querySelector("#story-list");
const statusCard = document.querySelector("#status-card");
const statusText = document.querySelector("#status-text");
const briefingHeading = document.querySelector("#briefing-heading");
const briefingMeta = document.querySelector("#briefing-meta");
const coverage = document.querySelector("#coverage");
const coverageText = document.querySelector("#coverage-text");
const coverageStatus = document.querySelector("#coverage-status");
const refreshButton = document.querySelector("#refresh-button");
const tabs = [...document.querySelectorAll(".feed-tab")];

let activeFeed = "mixed";
let activeRequest = 0;

function appendText(element, text) {
  element.append(document.createTextNode(text ?? ""));
  return element;
}

function formatDate(value) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "Time unavailable";
  return new Intl.DateTimeFormat("en-CA", { dateStyle: "medium", timeStyle: "short" }).format(date);
}

function relativeTime(value) {
  const timestamp = new Date(value).getTime();
  if (Number.isNaN(timestamp)) return "Date unavailable";
  const seconds = Math.round((timestamp - Date.now()) / 1000);
  const divisions = [
    [31_536_000, "year"], [2_592_000, "month"], [604_800, "week"],
    [86_400, "day"], [3_600, "hour"], [60, "minute"], [1, "second"],
  ];
  const formatter = new Intl.RelativeTimeFormat("en", { numeric: "auto" });
  for (const [amount, unit] of divisions) {
    if (Math.abs(seconds) >= amount || unit === "second") return formatter.format(Math.round(seconds / amount), unit);
  }
}

function safePublisherURL(value) {
  try {
    const url = new URL(value);
    return url.protocol === "https:" || url.protocol === "http:" ? url.href : null;
  } catch {
    return null;
  }
}

function createStory(item, index) {
  const story = document.createElement("li");
  story.className = "story";

  const number = appendText(document.createElement("span"), String(index + 1).padStart(2, "0"));
  number.className = "story-number";

  const body = document.createElement("article");
  const source = appendText(document.createElement("p"), item.source_name || "Unknown source");
  source.className = "story-source";
  const title = appendText(document.createElement("h3"), item.title || "Untitled story");
  const summary = appendText(document.createElement("p"), item.summary || item.why_relevant || "Open the publisher to read the full story.");
  summary.className = "story-summary";
  body.append(source, title, summary);

  const side = document.createElement("div");
  side.className = "story-side";
  const time = appendText(document.createElement("time"), relativeTime(item.published_at));
  time.className = "story-time";
  time.dateTime = item.published_at || "";
  time.title = formatDate(item.published_at);
  const publisherURL = safePublisherURL(item.url);
  const link = appendText(document.createElement(publisherURL ? "a" : "span"), publisherURL ? "Read at source ↗" : "Source unavailable");
  link.className = "story-link";
  if (publisherURL) {
    link.href = publisherURL;
    link.target = "_blank";
    link.rel = "noopener noreferrer";
  }
  side.append(time, link);

  story.append(number, body, side);
  return story;
}

function showCoverage(snapshot) {
  const notes = Array.isArray(snapshot.warnings) ? snapshot.warnings.filter(Boolean) : [];
  const state = snapshot.coverage?.status ?? "unknown";
  const selected = snapshot.coverage?.sources_selected;
  const current = snapshot.coverage?.sources_current;
  const base = Number.isFinite(selected) && Number.isFinite(current)
    ? `${current} of ${selected} selected sources contributed current items.`
    : "Northway reported the source coverage for this snapshot.";

  coverageText.textContent = notes.length ? `${base} ${notes.join(" ")}` : base;
  coverageStatus.textContent = state;
  coverage.hidden = false;
}

async function loadFeed(feed) {
  const requestNumber = ++activeRequest;
  activeFeed = feed;
  storyList.replaceChildren();
  storyList.setAttribute("aria-busy", "true");
  coverage.hidden = true;
  statusCard.hidden = false;
  statusCard.classList.remove("is-error");
  statusCard.querySelector(".loader").hidden = false;
  statusText.textContent = "Asking Northway for a fresh snapshot…";
  refreshButton.disabled = true;

  try {
    const response = await fetch("/api/news", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ feed, maxAgeHours: 168 }),
    });
    const result = await response.json();
    if (!response.ok) throw new Error(result.error || "Your briefing could not be retrieved.");
    if (requestNumber !== activeRequest) return;

    const { snapshot } = result;
    briefingHeading.textContent = result.feed.label;
    briefingMeta.textContent = `Generated ${formatDate(snapshot.generated_at)} · ${snapshot.ranking?.mode?.replaceAll("_", " ") || "ranked"}`;

    const items = Array.isArray(snapshot.items) ? snapshot.items : [];
    storyList.replaceChildren(...items.map(createStory));
    if (items.length === 0) {
      statusCard.hidden = false;
      statusCard.querySelector(".loader").hidden = true;
      statusText.textContent = "No current stories matched this feed. The empty result is preserved rather than padded.";
    } else {
      statusCard.hidden = true;
    }
    showCoverage(snapshot);
  } catch (error) {
    if (requestNumber !== activeRequest) return;
    briefingMeta.textContent = "Service unavailable";
    statusCard.hidden = false;
    statusCard.classList.add("is-error");
    statusCard.querySelector(".loader").hidden = true;
    statusText.textContent = error.message;
  } finally {
    if (requestNumber === activeRequest) {
      storyList.setAttribute("aria-busy", "false");
      refreshButton.disabled = false;
    }
  }
}

for (const tab of tabs) {
  tab.addEventListener("click", () => {
    for (const candidate of tabs) {
      const selected = candidate === tab;
      candidate.classList.toggle("is-active", selected);
      candidate.setAttribute("aria-selected", String(selected));
    }
    loadFeed(tab.dataset.feed);
  });
}

refreshButton.addEventListener("click", () => loadFeed(activeFeed));
loadFeed(activeFeed);
