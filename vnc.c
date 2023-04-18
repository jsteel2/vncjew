#include <rfb/rfbclient.h>
#include <jpeglib.h>

typedef struct
{
    char *client_name;
    int width, height;
    int auth_type;
} VNCInfo;

enum
{
    T_USERNAME, T_PASSWORD, T_AUTH_TYPE, T_UPDATES
};

void save_to_jpeg(rfbClient *client, char *filename)
{
    struct jpeg_compress_struct cinfo;
    struct jpeg_error_mgr jerr;

    cinfo.err = jpeg_std_error(&jerr);
    jpeg_create_compress(&cinfo);

    FILE *f = fopen(filename, "wb");
    if (!f)
    {
        jpeg_destroy_compress(&cinfo);
        return;
    }

    jpeg_stdio_dest(&cinfo, f);

    cinfo.image_width = client->width;
    cinfo.image_height = client->height;
    cinfo.input_components = 4;
    cinfo.in_color_space = JCS_EXT_RGBA;

    jpeg_set_defaults(&cinfo);
    jpeg_set_quality(&cinfo, 75, TRUE);

    jpeg_start_compress(&cinfo, TRUE);

    int row_stride = cinfo.image_width * 4;

    while (cinfo.next_scanline < cinfo.image_height)
    {
        JSAMPROW row_pointer = &client->frameBuffer[cinfo.next_scanline * row_stride];
        jpeg_write_scanlines(&cinfo, &row_pointer, 1);
    }

    jpeg_finish_compress(&cinfo);
    fclose(f);

    jpeg_destroy_compress(&cinfo);
}

rfbCredential *getcreds(rfbClient *client, int credential_type)
{
    if (credential_type != rfbCredentialTypeUser)
    {
        rfbClientErr("Unsupported authentication type\n");
        return NULL;
    }

    rfbCredential *c = malloc(sizeof(rfbCredential));
    c->userCredential.username = strdup(rfbClientGetClientData(client, (void *)T_USERNAME));
    c->userCredential.password = strdup(rfbClientGetClientData(client, (void *)T_PASSWORD));
    *(int *)rfbClientGetClientData(client, (void *)T_AUTH_TYPE) = 2;
    return c;
}

char *getpasswd(rfbClient *client)
{
    *(int *)rfbClientGetClientData(client, (void *)T_AUTH_TYPE) = 1;
    return strdup(rfbClientGetClientData(client, (void *)T_PASSWORD));
}

rfbBool malloc_fb(rfbClient *client)
{
    if (client->frameBuffer)
    {
        free(client->frameBuffer);
        client->frameBuffer = NULL;
    }
    client->frameBuffer = calloc(client->width * client->height, 4);
    int *updates = rfbClientGetClientData(client, (void *)T_UPDATES);
    *updates = 0;
    return TRUE;
}

void is_finished(rfbClient *client)
{
    size_t len = client->width * client->height;
    uint8_t *fb = client->frameBuffer;

    for (int i = 0; i < len; i++)
    {
        if (fb[i * 4 + 3] != 0xff) return;
    }

    int *updates = rfbClientGetClientData(client, (void *)T_UPDATES);
    (*updates)++;
    if (*updates < 2)
    {
        memset(fb, 0, len);
        SendFramebufferUpdateRequest(client, 0, 0, client->width, client->height, FALSE);
    }
}

void fb_update(rfbClient *client, int x, int y, int w, int h)
{
    uint8_t *where = client->frameBuffer + x * 4 + y * client->width * 4;
    int skip = client->width * 4;

    for (int i = 0; i < h; i++)
    {
        for (int j = 0; j < w; j++)
        {
            where[j * 4 + 3] = 0xff;
        }
        where += skip;
    }
}

char *screenshot(int timeout, char *host, int port, char *file, char *username, char *password, VNCInfo *ret)
{
    ret->client_name = NULL;
    rfbClient *client = rfbGetClient(8, 3, 4);

    rfbClientSetClientData(client, (void *)T_USERNAME, username);
    rfbClientSetClientData(client, (void *)T_PASSWORD, password);
    ret->auth_type = 0;
    rfbClientSetClientData(client, (void *)T_AUTH_TYPE, &ret->auth_type);

    client->GetCredential = getcreds;
    client->GetPassword = getpasswd;

    int updates = 0;
    rfbClientSetClientData(client, (void *)T_UPDATES, &updates);
    client->GotFrameBufferUpdate = fb_update;
    client->FinishedFrameBufferUpdate = is_finished;

    client->MallocFrameBuffer = malloc_fb;

    client->connectTimeout = timeout;
    client->readTimeout = timeout;

    client->canHandleNewFBSize = FALSE;
    if (!ConnectToRFBServer(client, host, port) || !InitialiseRFBConnection(client))
    {
        rfbClientCleanup(client);
        if (ret->auth_type != 0) return "Auth failed";
        return "Could not connect";
    }

    client->width = client->si.framebufferWidth;
    client->height = client->si.framebufferHeight;
    if (!client->MallocFrameBuffer(client))
    {
        rfbClientCleanup(client);
        return "Jeez";
    }

    if (!SetFormatAndEncodings(client)) goto err;
    client->updateRect.x = client->updateRect.y = 0;
    client->updateRect.w = client->width;
    client->updateRect.h = client->height;
    if (!SendFramebufferUpdateRequest(client, 0, 0, client->width, client->height, FALSE))
    {
        goto err;
    }

    ret->client_name = strdup(client->desktopName);
    ret->width = client->width;
    ret->height = client->height;

    time_t start_time = time(NULL);

    while (updates < 2 && time(NULL) - start_time < timeout)
    {
        int n = WaitForMessage(client, 100000);
        if (n < 0) goto err;
        if (n)
        {
            if (!HandleRFBServerMessage(client)) goto err;
        }
    }

    if (time(NULL) - start_time >= timeout && updates < 2) goto err;

    save_to_jpeg(client, file);
    free(client->frameBuffer);
    rfbClientCleanup(client);
    return NULL;

err:
    free(client->frameBuffer);
    rfbClientCleanup(client);
    return "Some error";
}

int main(int argc, char *argv[])
{
    VNCInfo ret;
    char *res = screenshot(atoi(argv[1]), argv[2], atoi(argv[3]), argv[4], argv[5], argv[6], &ret);
    if (res)
    {
        puts(res);
        return EXIT_FAILURE;
    }
    printf("%d\n%d\n%d\n%s", ret.auth_type, ret.width, ret.height, ret.client_name);
    return EXIT_SUCCESS;
}
