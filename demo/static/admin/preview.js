class PreviewComponent extends React.Component {
  constructor(props) {
    super(props);
    this.htmlRef = React.createRef();
  }

  async componentDidUpdate(prevProps) {
    const { entry } = this.props;
    if (this.props.entry !== prevProps && !this.previewInFlight) {
      this.previewInFlight = true;
      try {
        const payload = {
          path: entry.get("path"),
          data: entry.get("data").toJS(),
        };
        const resp = await fetch("/.netlify/functions/preview", {
          method: "POST",
          body: JSON.stringify(payload),
        });
        if (!resp.ok) {
          throw new Error(`Status ${resp.status}`);
        }
        const html = await resp.text();
        this.htmlRef.current.innerHTML = html;
      } catch (e) {
        console.error("Preview failed:", e);
      } finally {
        this.previewInFlight = false;
      }
    }
  }

  render() {
    return React.createElement("div", { ref: this.htmlRef });
  }
}

CMS.registerPreviewTemplate("post", PreviewComponent);
