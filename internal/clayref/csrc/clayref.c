#define _POSIX_C_SOURCE 200809L // getline.
// clayref is a reference harness for differential testing of glay against the
// upstream clay C implementation. It reads layout requests as JSON and writes
// the resulting render commands as JSON.
//
// Build: cc -std=c99 -O1 -I$CLAY_DIR -o clayref clayref.c -lm
//
// Usage:
//   clayref           one JSON request on stdin, one JSON response on stdout.
//   clayref -stream   newline delimited requests on stdin, one response line per request.
//
// See schema.go for the request/response schema.
#define CLAY_IMPLEMENTATION
#include "clay.h"

#include <math.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

// ---------------------------------------------------------------------------
// Minimal JSON DOM. Only what the schema needs.

typedef enum { J_NULL, J_FALSE, J_TRUE, J_NUM, J_STR, J_ARR, J_OBJ } jtype;

typedef struct jnode {
    jtype type;
    double num;
    char *str; // J_STR: decoded, NUL terminated.
    int len;   // J_STR: decoded length in bytes.
    char *key; // Member key when parent is J_OBJ.
    struct jnode *child, *next;
} jnode;

typedef struct {
    const char *p, *end;
    const char *err;
} jparser;

static jnode *jparse_value(jparser *jp);

static void jfree(jnode *n) {
    while (n) {
        jnode *next = n->next;
        jfree(n->child);
        free(n->str);
        free(n->key);
        free(n);
        n = next;
    }
}

static void jskip_ws(jparser *jp) {
    while (jp->p < jp->end && (*jp->p == ' ' || *jp->p == '\t' || *jp->p == '\n' || *jp->p == '\r')) {
        jp->p++;
    }
}

static int jhex(char c) {
    if (c >= '0' && c <= '9') return c - '0';
    if (c >= 'a' && c <= 'f') return c - 'a' + 10;
    if (c >= 'A' && c <= 'F') return c - 'A' + 10;
    return -1;
}

// jparse_string parses a string starting at the opening quote. Returns malloc'd decoded bytes.
static char *jparse_string(jparser *jp, int *outlen) {
    if (jp->p >= jp->end || *jp->p != '"') {
        jp->err = "expected string";
        return NULL;
    }
    jp->p++;
    size_t cap = 16, n = 0;
    char *buf = malloc(cap);
    while (jp->p < jp->end && *jp->p != '"') {
        unsigned char c = (unsigned char)*jp->p++;
        unsigned int cp = c;
        if (c == '\\') {
            if (jp->p >= jp->end) break;
            char e = *jp->p++;
            switch (e) {
            case '"': cp = '"'; break;
            case '\\': cp = '\\'; break;
            case '/': cp = '/'; break;
            case 'b': cp = '\b'; break;
            case 'f': cp = '\f'; break;
            case 'n': cp = '\n'; break;
            case 'r': cp = '\r'; break;
            case 't': cp = '\t'; break;
            case 'u': {
                if (jp->end - jp->p < 4) {
                    jp->err = "short unicode escape";
                    free(buf);
                    return NULL;
                }
                cp = 0;
                for (int i = 0; i < 4; i++) {
                    int h = jhex(jp->p[i]);
                    if (h < 0) {
                        jp->err = "bad unicode escape";
                        free(buf);
                        return NULL;
                    }
                    cp = cp << 4 | (unsigned int)h;
                }
                jp->p += 4;
                break;
            }
            default:
                jp->err = "bad escape";
                free(buf);
                return NULL;
            }
        }
        if (n + 4 >= cap) {
            cap *= 2;
            buf = realloc(buf, cap);
        }
        if (c == '\\' && cp >= 0x80) { // Encode escaped code point as UTF-8 (BMP only, as produced by encoding/json).
            if (cp < 0x800) {
                buf[n++] = (char)(0xC0 | (cp >> 6));
                buf[n++] = (char)(0x80 | (cp & 0x3F));
            } else {
                buf[n++] = (char)(0xE0 | (cp >> 12));
                buf[n++] = (char)(0x80 | ((cp >> 6) & 0x3F));
                buf[n++] = (char)(0x80 | (cp & 0x3F));
            }
        } else {
            buf[n++] = (char)cp;
        }
    }
    if (jp->p >= jp->end) {
        jp->err = "unterminated string";
        free(buf);
        return NULL;
    }
    jp->p++; // Closing quote.
    buf[n] = 0;
    *outlen = (int)n;
    return buf;
}

static jnode *jnew(jtype t) {
    jnode *n = calloc(1, sizeof(jnode));
    n->type = t;
    return n;
}

static jnode *jparse_container(jparser *jp, jtype t) {
    char closech = t == J_ARR ? ']' : '}';
    jnode *n = jnew(t);
    jnode **tail = &n->child;
    jp->p++; // Opening bracket.
    jskip_ws(jp);
    if (jp->p < jp->end && *jp->p == closech) {
        jp->p++;
        return n;
    }
    for (;;) {
        char *key = NULL;
        if (t == J_OBJ) {
            jskip_ws(jp);
            int keylen;
            key = jparse_string(jp, &keylen);
            if (!key) goto fail;
            jskip_ws(jp);
            if (jp->p >= jp->end || *jp->p != ':') {
                free(key);
                jp->err = "expected colon";
                goto fail;
            }
            jp->p++;
        }
        jnode *v = jparse_value(jp);
        if (!v) {
            free(key);
            goto fail;
        }
        v->key = key;
        *tail = v;
        tail = &v->next;
        jskip_ws(jp);
        if (jp->p < jp->end && *jp->p == ',') {
            jp->p++;
            continue;
        }
        if (jp->p < jp->end && *jp->p == closech) {
            jp->p++;
            return n;
        }
        jp->err = "expected comma or closing bracket";
        goto fail;
    }
fail:
    jfree(n);
    return NULL;
}

static jnode *jparse_value(jparser *jp) {
    jskip_ws(jp);
    if (jp->p >= jp->end) {
        jp->err = "unexpected end of input";
        return NULL;
    }
    char c = *jp->p;
    if (c == '{') return jparse_container(jp, J_OBJ);
    if (c == '[') return jparse_container(jp, J_ARR);
    if (c == '"') {
        jnode *n = jnew(J_STR);
        n->str = jparse_string(jp, &n->len);
        if (!n->str) {
            free(n);
            return NULL;
        }
        return n;
    }
    if (jp->end - jp->p >= 4 && strncmp(jp->p, "null", 4) == 0) {
        jp->p += 4;
        return jnew(J_NULL);
    }
    if (jp->end - jp->p >= 4 && strncmp(jp->p, "true", 4) == 0) {
        jp->p += 4;
        return jnew(J_TRUE);
    }
    if (jp->end - jp->p >= 5 && strncmp(jp->p, "false", 5) == 0) {
        jp->p += 5;
        return jnew(J_FALSE);
    }
    char *numend;
    double v = strtod(jp->p, &numend);
    if (numend == jp->p) {
        jp->err = "unexpected character";
        return NULL;
    }
    jp->p = numend;
    jnode *n = jnew(J_NUM);
    n->num = v;
    return n;
}

static jnode *jget(const jnode *obj, const char *key) {
    if (!obj || obj->type != J_OBJ) return NULL;
    for (jnode *c = obj->child; c; c = c->next) {
        if (strcmp(c->key, key) == 0) return c;
    }
    return NULL;
}

static jnode *jat(const jnode *arr, int i) {
    if (!arr || arr->type != J_ARR) return NULL;
    jnode *c = arr->child;
    for (; c && i > 0; i--) c = c->next;
    return c;
}

static double jnum(const jnode *n, double def) { return n && n->type == J_NUM ? n->num : def; }
static int jbool(const jnode *n) { return n && n->type == J_TRUE; }
static int jstreq(const jnode *n, const char *s) { return n && n->type == J_STR && strcmp(n->str, s) == 0; }

// ---------------------------------------------------------------------------
// Request decoding into clay structures.

static Clay_Color dec_color(const jnode *n) {
    return (Clay_Color){
        (float)jnum(jat(n, 0), 0), (float)jnum(jat(n, 1), 0),
        (float)jnum(jat(n, 2), 0), (float)jnum(jat(n, 3), 0)};
}

static Clay_SizingAxis dec_axis(const jnode *n) {
    Clay_SizingAxis ax = {0};
    const jnode *t = jget(n, "type");
    if (jstreq(t, "grow")) ax.type = CLAY__SIZING_TYPE_GROW;
    else if (jstreq(t, "percent")) ax.type = CLAY__SIZING_TYPE_PERCENT;
    else if (jstreq(t, "fixed")) ax.type = CLAY__SIZING_TYPE_FIXED;
    else ax.type = CLAY__SIZING_TYPE_FIT;
    if (ax.type == CLAY__SIZING_TYPE_PERCENT) {
        ax.size.percent = (float)jnum(jget(n, "pct"), 0);
    } else {
        ax.size.minMax.min = (float)jnum(jget(n, "min"), 0);
        ax.size.minMax.max = (float)jnum(jget(n, "max"), 0);
    }
    return ax;
}

static Clay_LayoutConfig dec_layout(const jnode *n) {
    Clay_LayoutConfig l = {0};
    if (!n) return l;
    l.sizing.width = dec_axis(jget(n, "w"));
    l.sizing.height = dec_axis(jget(n, "h"));
    const jnode *pad = jget(n, "padding");
    l.padding = (Clay_Padding){
        (uint16_t)jnum(jat(pad, 0), 0), (uint16_t)jnum(jat(pad, 1), 0),
        (uint16_t)jnum(jat(pad, 2), 0), (uint16_t)jnum(jat(pad, 3), 0)};
    l.childGap = (uint16_t)jnum(jget(n, "gap"), 0);
    const jnode *ax = jget(n, "ax");
    if (jstreq(ax, "right")) l.childAlignment.x = CLAY_ALIGN_X_RIGHT;
    else if (jstreq(ax, "center")) l.childAlignment.x = CLAY_ALIGN_X_CENTER;
    const jnode *ay = jget(n, "ay");
    if (jstreq(ay, "bottom")) l.childAlignment.y = CLAY_ALIGN_Y_BOTTOM;
    else if (jstreq(ay, "center")) l.childAlignment.y = CLAY_ALIGN_Y_CENTER;
    if (jstreq(jget(n, "dir"), "ttb")) l.layoutDirection = CLAY_TOP_TO_BOTTOM;
    return l;
}

static uint32_t dec_parent_id(const jnode *fl) {
    const jnode *sid = jget(fl, "parentSid");
    if (sid) {
        Clay_String s = {.isStaticallyAllocated = false, .length = sid->len, .chars = sid->str};
        return Clay__HashString(s, 0).id;
    }
    return (uint32_t)jnum(jget(fl, "parentId"), 0);
}

static Clay_ElementDeclaration dec_decl(const jnode *n) {
    Clay_ElementDeclaration d = {0};
    if (!n) return d;
    d.layout = dec_layout(jget(n, "layout"));
    d.backgroundColor = dec_color(jget(n, "bg"));
    const jnode *r = jget(n, "radius");
    d.cornerRadius = (Clay_CornerRadius){
        (float)jnum(jat(r, 0), 0), (float)jnum(jat(r, 1), 0),
        (float)jnum(jat(r, 2), 0), (float)jnum(jat(r, 3), 0)};
    d.aspectRatio.aspectRatio = (float)jnum(jget(n, "aspect"), 0);
    d.image.imageData = (void *)(uintptr_t)jnum(jget(n, "image"), 0);
    d.custom.customData = (void *)(uintptr_t)jnum(jget(n, "custom"), 0);
    d.userData = (void *)(uintptr_t)jnum(jget(n, "user"), 0);
    const jnode *fl = jget(n, "floating");
    if (fl) {
        const jnode *off = jget(fl, "offset"), *exp = jget(fl, "expand"), *at = jget(fl, "attach");
        d.floating.offset = (Clay_Vector2){(float)jnum(jat(off, 0), 0), (float)jnum(jat(off, 1), 0)};
        d.floating.expand = (Clay_Dimensions){(float)jnum(jat(exp, 0), 0), (float)jnum(jat(exp, 1), 0)};
        d.floating.parentId = dec_parent_id(fl);
        d.floating.zIndex = (int16_t)jnum(jget(fl, "z"), 0);
        d.floating.attachPoints.element = (Clay_FloatingAttachPointType)jnum(jat(at, 0), 0);
        d.floating.attachPoints.parent = (Clay_FloatingAttachPointType)jnum(jat(at, 1), 0);
        if (jstreq(jget(fl, "capture"), "passthrough")) d.floating.pointerCaptureMode = CLAY_POINTER_CAPTURE_MODE_PASSTHROUGH;
        const jnode *to = jget(fl, "attachTo");
        if (jstreq(to, "parent")) d.floating.attachTo = CLAY_ATTACH_TO_PARENT;
        else if (jstreq(to, "id")) d.floating.attachTo = CLAY_ATTACH_TO_ELEMENT_WITH_ID;
        else if (jstreq(to, "root")) d.floating.attachTo = CLAY_ATTACH_TO_ROOT;
        if (jstreq(jget(fl, "clipTo"), "parent")) d.floating.clipTo = CLAY_CLIP_TO_ATTACHED_PARENT;
    }
    const jnode *cl = jget(n, "clip");
    if (cl) {
        const jnode *off = jget(cl, "offset");
        d.clip.horizontal = jbool(jget(cl, "h"));
        d.clip.vertical = jbool(jget(cl, "v"));
        d.clip.childOffset = (Clay_Vector2){(float)jnum(jat(off, 0), 0), (float)jnum(jat(off, 1), 0)};
    }
    const jnode *b = jget(n, "border");
    if (b) {
        const jnode *w = jget(b, "width");
        d.border.color = dec_color(jget(b, "color"));
        d.border.width = (Clay_BorderWidth){
            (uint16_t)jnum(jat(w, 0), 0), (uint16_t)jnum(jat(w, 1), 0), (uint16_t)jnum(jat(w, 2), 0),
            (uint16_t)jnum(jat(w, 3), 0), (uint16_t)jnum(jat(w, 4), 0)};
    }
    return d;
}

static Clay_TextElementConfig dec_textcfg(const jnode *n) {
    Clay_TextElementConfig c = {0};
    if (!n) return c;
    c.userData = (void *)(uintptr_t)jnum(jget(n, "user"), 0);
    c.textColor = dec_color(jget(n, "color"));
    c.fontId = (uint16_t)jnum(jget(n, "font"), 0);
    c.fontSize = (uint16_t)jnum(jget(n, "size"), 0);
    c.letterSpacing = (uint16_t)jnum(jget(n, "spacing"), 0);
    c.lineHeight = (uint16_t)jnum(jget(n, "lineH"), 0);
    const jnode *wrap = jget(n, "wrap");
    if (jstreq(wrap, "newlines")) c.wrapMode = CLAY_TEXT_WRAP_NEWLINES;
    else if (jstreq(wrap, "none")) c.wrapMode = CLAY_TEXT_WRAP_NONE;
    const jnode *al = jget(n, "align");
    if (jstreq(al, "center")) c.textAlignment = CLAY_TEXT_ALIGN_CENTER;
    else if (jstreq(al, "right")) c.textAlignment = CLAY_TEXT_ALIGN_RIGHT;
    return c;
}

static Clay_String mkstr(const jnode *n) {
    return (Clay_String){.isStaticallyAllocated = false, .length = n ? n->len : 0, .chars = n ? n->str : ""};
}

// ---------------------------------------------------------------------------
// Output.

static void put_float(FILE *w, float f) {
    if (isnan(f)) fputs("\"NaN\"", w);
    else if (isinf(f)) fputs(f > 0 ? "\"+Inf\"" : "\"-Inf\"", w);
    else fprintf(w, "%.9g", (double)f);
}

static void put_floats(FILE *w, const float *f, int n) {
    fputc('[', w);
    for (int i = 0; i < n; i++) {
        if (i) fputc(',', w);
        put_float(w, f[i]);
    }
    fputc(']', w);
}

static void put_color(FILE *w, const char *key, Clay_Color c) {
    float f[4] = {c.r, c.g, c.b, c.a};
    fprintf(w, ",\"%s\":", key);
    put_floats(w, f, 4);
}

static void put_radius(FILE *w, Clay_CornerRadius r) {
    float f[4] = {r.topLeft, r.topRight, r.bottomLeft, r.bottomRight};
    fputs(",\"radius\":", w);
    put_floats(w, f, 4);
}

static void put_bb(FILE *w, Clay_BoundingBox bb) {
    float f[4] = {bb.x, bb.y, bb.width, bb.height};
    put_floats(w, f, 4);
}

static void put_jstring(FILE *w, const char *s, int n) {
    fputc('"', w);
    for (int i = 0; i < n; i++) {
        unsigned char c = (unsigned char)s[i];
        if (c == '"' || c == '\\') fprintf(w, "\\%c", c);
        else if (c < 0x20) fprintf(w, "\\u%04x", c);
        else fputc(c, w);
    }
    fputc('"', w);
}

static const char *cmdname(Clay_RenderCommandType t) {
    switch (t) {
    case CLAY_RENDER_COMMAND_TYPE_NONE: return "none";
    case CLAY_RENDER_COMMAND_TYPE_RECTANGLE: return "rectangle";
    case CLAY_RENDER_COMMAND_TYPE_BORDER: return "border";
    case CLAY_RENDER_COMMAND_TYPE_TEXT: return "text";
    case CLAY_RENDER_COMMAND_TYPE_IMAGE: return "image";
    case CLAY_RENDER_COMMAND_TYPE_SCISSOR_START: return "scissor_start";
    case CLAY_RENDER_COMMAND_TYPE_SCISSOR_END: return "scissor_end";
    case CLAY_RENDER_COMMAND_TYPE_OVERLAY_COLOR_START: return "overlay_color_start";
    case CLAY_RENDER_COMMAND_TYPE_OVERLAY_COLOR_END: return "overlay_color_end";
    case CLAY_RENDER_COMMAND_TYPE_CUSTOM: return "custom";
    }
    return "unknown";
}

static void put_command(FILE *w, const Clay_RenderCommand *c) {
    fprintf(w, "{\"type\":\"%s\",\"bb\":", cmdname(c->commandType));
    put_bb(w, c->boundingBox);
    fprintf(w, ",\"id\":%u,\"z\":%d,\"user\":%llu", c->id, (int)c->zIndex, (unsigned long long)(uintptr_t)c->userData);
    const Clay_RenderData *d = &c->renderData;
    switch (c->commandType) {
    case CLAY_RENDER_COMMAND_TYPE_RECTANGLE:
        put_color(w, "color", d->rectangle.backgroundColor);
        put_radius(w, d->rectangle.cornerRadius);
        break;
    case CLAY_RENDER_COMMAND_TYPE_BORDER: {
        Clay_BorderWidth bw = d->border.width;
        put_color(w, "color", d->border.color);
        put_radius(w, d->border.cornerRadius);
        fprintf(w, ",\"width\":[%u,%u,%u,%u,%u]", bw.left, bw.right, bw.top, bw.bottom, bw.betweenChildren);
        break;
    }
    case CLAY_RENDER_COMMAND_TYPE_TEXT: {
        const Clay_TextRenderData *t = &d->text;
        fputs(",\"text\":", w);
        put_jstring(w, t->stringContents.chars, t->stringContents.length);
        fprintf(w, ",\"offset\":%d", (int)(t->stringContents.chars - t->stringContents.baseChars));
        put_color(w, "color", t->textColor);
        fprintf(w, ",\"font\":%u,\"size\":%u,\"spacing\":%u,\"lineH\":%u", t->fontId, t->fontSize, t->letterSpacing, t->lineHeight);
        break;
    }
    case CLAY_RENDER_COMMAND_TYPE_IMAGE:
        put_color(w, "color", d->image.backgroundColor);
        put_radius(w, d->image.cornerRadius);
        fprintf(w, ",\"image\":%llu", (unsigned long long)(uintptr_t)d->image.imageData);
        break;
    case CLAY_RENDER_COMMAND_TYPE_SCISSOR_START:
    case CLAY_RENDER_COMMAND_TYPE_SCISSOR_END:
        fprintf(w, ",\"clip\":[%s,%s]", d->clip.horizontal ? "true" : "false", d->clip.vertical ? "true" : "false");
        break;
    case CLAY_RENDER_COMMAND_TYPE_OVERLAY_COLOR_START:
    case CLAY_RENDER_COMMAND_TYPE_OVERLAY_COLOR_END:
        put_color(w, "color", d->overlayColor.color);
        break;
    case CLAY_RENDER_COMMAND_TYPE_CUSTOM:
        put_color(w, "color", d->custom.backgroundColor);
        put_radius(w, d->custom.cornerRadius);
        fprintf(w, ",\"custom\":%llu", (unsigned long long)(uintptr_t)d->custom.customData);
        break;
    default:
        break;
    }
    fputc('}', w);
}

// ---------------------------------------------------------------------------
// Running requests.

static const char *errname(Clay_ErrorType t) {
    switch (t) {
    case CLAY_ERROR_TYPE_TEXT_MEASUREMENT_FUNCTION_NOT_PROVIDED: return "text_measurement_function_not_provided";
    case CLAY_ERROR_TYPE_ARENA_CAPACITY_EXCEEDED: return "arena_capacity_exceeded";
    case CLAY_ERROR_TYPE_ELEMENTS_CAPACITY_EXCEEDED: return "elements_capacity_exceeded";
    case CLAY_ERROR_TYPE_TEXT_MEASUREMENT_CAPACITY_EXCEEDED: return "text_measurement_capacity_exceeded";
    case CLAY_ERROR_TYPE_DUPLICATE_ID: return "duplicate_id";
    case CLAY_ERROR_TYPE_FLOATING_CONTAINER_PARENT_NOT_FOUND: return "floating_container_parent_not_found";
    case CLAY_ERROR_TYPE_PERCENTAGE_OVER_1: return "percentage_over_1";
    case CLAY_ERROR_TYPE_INTERNAL_ERROR: return "internal_error";
    case CLAY_ERROR_TYPE_UNBALANCED_OPEN_CLOSE: return "unbalanced_open_close";
    case CLAY_ERROR_TYPE_HASH_MAP_CAPACITY_EXCEEDED: return "hash_map_capacity_exceeded";
    }
    return "unknown";
}

#define MAX_ERRS 64
static Clay_ErrorType g_errs[MAX_ERRS];
static int g_nerrs;
static float g_charW, g_lineH;
static void *g_arena;

static void on_error(Clay_ErrorData e) {
    if (g_nerrs < MAX_ERRS) g_errs[g_nerrs++] = e.errorType;
}

// measure is the deterministic monospace text measurer shared with the Go side (see schema.go).
static Clay_Dimensions measure(Clay_StringSlice text, Clay_TextElementConfig *cfg, void *userData) {
    (void)userData;
    float n = (float)text.length;
    return (Clay_Dimensions){n * (g_charW + (float)cfg->letterSpacing), g_lineH};
}

static Clay_ElementId open_id(const jnode *op, int *hasID) {
    const jnode *sid = jget(op, "sid"), *sidi = jget(op, "sidi"), *rawid = jget(op, "id");
    uint32_t seed = jbool(jget(op, "local")) ? Clay_GetOpenElementId() : 0;
    *hasID = 1;
    if (sid) return Clay__HashString(mkstr(sid), seed);
    if (sidi) return Clay__HashStringWithOffset(mkstr(jat(sidi, 0)), (uint32_t)jnum(jat(sidi, 1), 0), seed);
    if (rawid) return (Clay_ElementId){.id = (uint32_t)jnum(rawid, 0)};
    *hasID = 0;
    return (Clay_ElementId){0};
}

static void run_frame(const jnode *frame, FILE *w) {
    const jnode *fw = jget(frame, "w"), *fh = jget(frame, "h");
    if (fw || fh) {
        Clay_Dimensions dims = Clay_GetLayoutDimensions();
        Clay_SetLayoutDimensions((Clay_Dimensions){(float)jnum(fw, dims.width), (float)jnum(fh, dims.height)});
    }
    g_nerrs = 0;
    Clay_BeginLayout();
    int depth = 0;
    for (const jnode *op = jat(jget(frame, "ops"), 0); op; op = op->next) {
        const jnode *kind = jget(op, "op");
        if (jstreq(kind, "open")) {
            int hasID;
            Clay_ElementId id = open_id(op, &hasID);
            if (hasID) Clay__OpenElementWithId(id);
            else Clay__OpenElement();
            Clay__ConfigureOpenElement(dec_decl(jget(op, "decl")));
            depth++;
        } else if (jstreq(kind, "text")) {
            Clay__OpenTextElement(mkstr(jget(op, "text")), dec_textcfg(jget(op, "cfg")));
        } else if (jstreq(kind, "close")) {
            if (depth > 0) { // Closing the root element is not a valid use of the API, skip it.
                Clay__CloseElement();
                depth--;
            }
        }
    }
    Clay_RenderCommandArray cmds = Clay_EndLayout(0);

    fputs("{\"errors\":[", w);
    for (int i = 0; i < g_nerrs; i++) {
        if (i) fputc(',', w);
        fprintf(w, "\"%s\"", errname(g_errs[i]));
    }
    fputs("],\"commands\":[", w);
    for (int i = 0; i < cmds.length; i++) {
        if (i) fputc(',', w);
        put_command(w, &cmds.internalArray[i]);
    }
    fputs("],\"query\":[", w);
    int i = 0;
    for (const jnode *q = jat(jget(frame, "query"), 0); q; q = q->next, i++) {
        Clay_ElementId id = Clay__HashString(mkstr(q), 0);
        Clay_ElementData ed = Clay_GetElementData(id);
        Clay_ScrollContainerData sd = Clay_GetScrollContainerData(id);
        if (i) fputc(',', w);
        fprintf(w, "{\"id\":%u,\"found\":%s,\"bb\":", id.id, ed.found ? "true" : "false");
        put_bb(w, ed.boundingBox);
        fprintf(w, ",\"scrollFound\":%s,\"scrollDims\":", sd.found ? "true" : "false");
        float dims[4] = {sd.scrollContainerDimensions.width, sd.scrollContainerDimensions.height,
                         sd.contentDimensions.width, sd.contentDimensions.height};
        put_floats(w, dims, 4);
        fputc('}', w);
    }
    fputs("]}", w);
}

static void run_request(const jnode *req, FILE *w) {
    int maxElements = (int)jnum(jget(req, "maxElements"), 8192);
    int maxWords = (int)jnum(jget(req, "maxWords"), maxElements * 2);
    const jnode *layout = jget(req, "layout"), *meas = jget(req, "measure");
    g_charW = (float)jnum(jget(meas, "charW"), 8);
    g_lineH = (float)jnum(jget(meas, "lineH"), 16);

    // Clay returns pointers to these global defaults on lookup misses and capacity overflows, and
    // sometimes writes through them. Reset them so each request is independent of previous ones.
    Clay_LayoutElementHashMapItem_DEFAULT = (Clay_LayoutElementHashMapItem){0};
    Clay__ScrollContainerDataInternal_DEFAULT = (Clay__ScrollContainerDataInternal){0};
    Clay_LayoutElement_DEFAULT = (Clay_LayoutElement){0};
    Clay__MeasureTextCacheItem_DEFAULT = (Clay__MeasureTextCacheItem){0};
    Clay_SetMaxElementCount(maxElements);
    Clay_SetMaxMeasureTextCacheWordCount(maxWords);
    uint32_t size = Clay_MinMemorySize();
    void *mem = malloc(size);
    Clay_Arena arena = Clay_CreateArenaWithCapacityAndMemory(size, mem);
    Clay_Dimensions dims = {(float)jnum(jget(layout, "w"), 800), (float)jnum(jget(layout, "h"), 600)};
    Clay_Initialize(arena, dims, (Clay_ErrorHandler){on_error, 0});
    free(g_arena); // Previous context was read by Clay_Initialize; safe to release now.
    g_arena = mem;
    Clay_SetMeasureTextFunction(measure, 0);
    Clay_SetCullingEnabled(!jbool(jget(req, "disableCulling")));

    fputs("{\"frames\":[", w);
    int i = 0;
    for (const jnode *f = jat(jget(req, "frames"), 0); f; f = f->next, i++) {
        if (i) fputc(',', w);
        run_frame(f, w);
    }
    fputs("]}", w);
}

static int handle(const char *buf, size_t n, FILE *w) {
    jparser jp = {buf, buf + n, NULL};
    jnode *req = jparse_value(&jp);
    if (!req) {
        fprintf(w, "{\"fatal\":\"json: %s at byte %ld\"}", jp.err, (long)(jp.p - buf));
        return 1;
    }
    run_request(req, w);
    jfree(req);
    return 0;
}

int main(int argc, char **argv) {
    int stream = argc > 1 && strcmp(argv[1], "-stream") == 0;
    if (stream) {
        char *line = NULL;
        size_t cap = 0;
        ssize_t n;
        while ((n = getline(&line, &cap, stdin)) > 0) {
            if (n == 1 && line[0] == '\n') continue;
            handle(line, (size_t)n, stdout);
            fputc('\n', stdout);
            fflush(stdout);
        }
        free(line);
        return 0;
    }
    size_t cap = 1 << 16, n = 0;
    char *buf = malloc(cap);
    size_t r;
    while ((r = fread(buf + n, 1, cap - n, stdin)) > 0) {
        n += r;
        if (n == cap) {
            cap *= 2;
            buf = realloc(buf, cap);
        }
    }
    int rc = handle(buf, n, stdout);
    fputc('\n', stdout);
    free(buf);
    return rc;
}
